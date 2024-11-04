package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/tcp-clients/client"
	"example.com/tcp-clients/game"
	"example.com/tcp-clients/words"
)

const (
	commandCountClients            = "/count"
	commandCountClientsDesc        = "number of connected clients to the server"
	commandSpecialCommands         = "/commands"
	commandSpecialCommandsDesc     = "show special commands"
	commandGame                    = "/game"
	commandGameDesc                = "play guess a word with other users (english words)"
	commandEndGame                 = "/endgame"
	commandEndGameDesc             = "end game session"
	maxLenMsg                      = 200
	flagMaxConnectedClients        = "max-connected-clients"
	flagMaxConnectedClientsDefault = 100
	flagMaxConnectedClientsDesc    = "maximum of connected clients at the same time"
	flagListenAddr                 = "listen-addr"
	flagListenAddrDefault          = "localhost:8080"
	flagListenAddrDesc             = "server listen address (ex : listen-addr=localhost:8080)"
)

// app is an instance grouping functionalities of the app.
// Used for dependency injection.
type app struct {
	logger              *slog.Logger
	clients             *client.Clients
	guessWordGameEngine *game.GuessWordGameEngine
}

// broadcastMessage broadcasts a message to all other clients except the one who sent the broadcastMessage
// and the ones who are inside the waiting queue.
//
// clientID : client ID of the client who sent the message.
func (app *app) broadcastMessage(message string, clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.clients.Mu.RLock()
	for k, v := range app.clients.Clients {
		if k != clientID && !slices.Contains(app.clients.WaitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.clients.Mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[%v][client #%v] %v\n", client.RemoteAddr().String(), clientID, message)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastServerMessageClientLeft broadcasts an official server message to all
// other clients and clients not inside the waiting queue that the client with clientID left.
//
// clientID : client ID of the client who left.
func (app *app) broadcastServerMessageClientLeft(clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.clients.Mu.RLock()
	for k, v := range app.clients.Clients {
		if k != clientID && !slices.Contains(app.clients.WaitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.clients.Mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[server] client #%v left the server.\n\n", clientID)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastGameStarted broadcasts that a game started.
func (app *app) broadcastGameStarted() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.clients.Mu.RLock()
	for k, v := range app.clients.Clients {
		if !slices.Contains(app.clients.WaitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.clients.Mu.RUnlock()

	app.guessWordGameEngine.Mu.RLock()
	defer app.guessWordGameEngine.Mu.RUnlock()
	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game started! Try to guess to word\n"+
			"The word has %d letters.\n"+
			"The word starts with: %v\n\n",
			len(app.guessWordGameEngine.Word),
			string(app.guessWordGameEngine.Word[0]))
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastGameEnded broadcasts that the game ended.
func (app *app) broadcastGameEnded() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.clients.Mu.RLock()
	for k, v := range app.clients.Clients {
		if !slices.Contains(app.clients.WaitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.clients.Mu.RUnlock()

	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game ended! The word was : %v\n"+
			"Have a good day!\n\n",
			app.guessWordGameEngine.Word)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastQueueStatusToClientsWaiting broadcasts the status of the waiting queue
// to the clients who are in the waiting queue.
func (app *app) broadcastQueueStatusToClientsWaiting() {
	clientsInWaitingQueue := make(map[int64]net.Conn)

	app.clients.Mu.RLock()
	for k, v := range app.clients.Clients {
		idx := slices.Index(app.clients.WaitingQueueIDs, k)
		if idx != -1 {
			clientsInWaitingQueue[int64(idx)] = v
		}
	}
	app.clients.Mu.RUnlock()

	for k, client := range clientsInWaitingQueue {
		msg := fmt.Sprintf("You are in the position %d in the queue.\n\n", k+1)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastServerShutdown broadcast to the clients that the server is shutting down.
func (app *app) broadcastServerShutdown() {
	app.clients.Mu.RLock()
	defer app.clients.Mu.RUnlock()
	for _, client := range app.clients.Clients {
		msg := "[server] the server is shutting down. Goodbye!\n"
		if _, err := client.Write([]byte(msg)); err != nil {
			app.logger.Error("error writing shutdown message to client", slog.Any("error", err))
		}
	}
}

// handleConnection handles a connection to the server.
func (app *app) handleConnection(ctx context.Context, conn net.Conn) {
	clientID := app.clients.NextID()

	app.clients.Add(clientID, conn)
	app.logger.InfoContext(ctx, "got a connection:",
		slog.Any("client_id", clientID),
		slog.Any("remote_addr", conn.RemoteAddr().String()))

	defer func() {
		closeErr := conn.Close()
		app.clients.Remove(clientID)
		app.broadcastServerMessageClientLeft(clientID)
		app.broadcastQueueStatusToClientsWaiting()
		if closeErr != nil {
			app.logger.Error("error closing connection", slog.Any("error", closeErr))
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	_, err := rw.WriteString(showWelcomeMsg(app, clientID))
	if err != nil {
		app.logger.ErrorContext(ctx, "error writing to client", slog.Any("error", err))
		return
	}

	// Flush to send the welcome message directly to the client.
	err = rw.Flush()
	if err != nil {
		app.logger.ErrorContext(ctx, "error flushing data", slog.Any("error", err))
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if deadlineErr := conn.SetDeadline(time.Now().Add(time.Second * 1)); deadlineErr != nil {
				app.logger.ErrorContext(ctx, "error setting read deadline", slog.Any("error", deadlineErr))
				return
			}
			var message string

			message, err = rw.ReadString('\n')
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}

				if errors.Is(err, io.EOF) {
					app.logger.InfoContext(ctx, "client disconnected",
						slog.Any("client_id", clientID),
						slog.Any("remote_addr", conn.RemoteAddr().String()))
				} else {
					app.logger.ErrorContext(ctx, "error reading client message",
						slog.Any("client_id", clientID),
						slog.Any("error", err))
				}
				return
			}

			trimmedMessage := strings.TrimSpace(message)
			app.logger.InfoContext(ctx, "received message",
				slog.String("remote_addr", conn.RemoteAddr().String()),
				slog.Any("client_id", clientID),
				slog.String("message", trimmedMessage))

			handleMessage(trimmedMessage, app, rw, clientID)

			// Flush the buffer to ensure data is sent.
			err = rw.Flush()
			if err != nil {
				app.logger.ErrorContext(ctx, "error flushing data", slog.Any("error", err))
				return
			}
		}
	}
}

func main() {
	maxConnectedClients := flag.Int(
		flagMaxConnectedClients,
		flagMaxConnectedClientsDefault,
		flagMaxConnectedClientsDesc)

	listenAddr := flag.String(
		flagListenAddr,
		flagListenAddrDefault,
		flagListenAddrDesc)

	flag.Parse()

	if *maxConnectedClients <= 1 {
		panic("cannot have 1 or less as" + flagMaxConnectedClients)
	}

	if *listenAddr == "" {
		panic("enter a valid" + flagListenAddr)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	app := &app{
		logger: logger,
		clients: &client.Clients{
			Clients:             make(map[int64]net.Conn),
			MaxConnectedClients: *maxConnectedClients,
			WaitingQueueIDs:     make([]int64, 0, *maxConnectedClients),
		},
		guessWordGameEngine: &game.GuessWordGameEngine{
			Word:    "",
			Tries:   0,
			Started: false,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigsChan := make(chan os.Signal, 1)
	signal.Notify(sigsChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	ln, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		app.logger.Error("failed to listen", slog.Any("error", err))
		panic(err)
	}
	app.logger.Info("tcp server listening for connections...")

	// Goroutine to listen for signals.
	go func() {
		<-sigsChan
		app.logger.Info("Shutting down the server...")
		app.broadcastServerShutdown()
		cancel()
		if closeErr := ln.Close(); closeErr != nil {
			app.logger.Error(
				"error while closing the tcp connection",
				slog.Any("error", err))
		}
	}()

	var wg sync.WaitGroup

AcceptLoop:
	for {
		var conn net.Conn

		conn, err = ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				break AcceptLoop
			default:
				app.logger.Warn("cannot accept connection", slog.Any("error", err))
				continue
			}
		}

		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			app.handleConnection(ctx, c)
		}(conn)
	}

	wg.Wait()
	app.logger.Info("server shutdown")
}

// handleMessage handles the message send to the server accordingly.
func handleMessage(message string, app *app, rw *bufio.ReadWriter, clientID int64) {
	idxInsideWaitingQueue := app.clients.IndexClientInWaitingQueue(clientID)

	switch {
	case len(message) == 0 || len(message) > maxLenMsg:
		msg := fmt.Sprintf("[server] Message length must be between 1 and %d characters.\n", maxLenMsg)
		if _, err := rw.WriteString(msg); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
		return
	case idxInsideWaitingQueue != -1:
		return
	case message == commandCountClients:
		count := fmt.Sprintf("[server] %d\n\n", app.clients.Count())
		if _, err := rw.WriteString(count); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandSpecialCommands:
		if _, err := rw.WriteString(showSpecialCommands()); err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandGame:
		if !app.guessWordGameEngine.Started {
			err := startNewGuessWordGame(app, rw)
			if err != nil {
				app.logger.Error("Error while trying to initialize the guess word game", slog.Any("error", err))
				return
			}

			app.broadcastGameStarted()
		} else {
			_, err := rw.WriteString("A game already started!\n\n")
			if err != nil {
				app.logger.Error("error writing to client", slog.Any("error", err))
				return
			}
		}

	case message == commandEndGame:
		if app.guessWordGameEngine.EndGame() {
			app.broadcastGameEnded()
		}

	case message[0] == '/':
		msg := fmt.Sprintf("[server] unknown command '%v'\n\n", message)
		_, err := rw.WriteString(msg)
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}

	default:
		if !app.guessWordGameEngine.Started {
			app.broadcastMessage(message, clientID)
			return
		}

		err := handleGuessWordGame(app, message, rw, clientID)
		if err != nil {
			app.logger.Error("Error handling the guess a word game", slog.Any("error", err))
			return
		}
	}
}

// handleGuessWordGame handles the logic of the guess word game.
func handleGuessWordGame(app *app, message string, rw *bufio.ReadWriter, clientID int64) error {
	wordFound := app.guessWordGameEngine.CheckWord(message)
	if wordFound {
		msg := fmt.Sprintf("Congratulations! You guessed the word '%s' correctly.\n", app.guessWordGameEngine.Word)
		if _, err := rw.WriteString(msg); err != nil {
			return err
		}

		app.guessWordGameEngine.EndGame()

		msg = fmt.Sprintf("Client #%d guessed the word '%s' correctly!", clientID, app.guessWordGameEngine.Word)
		app.broadcastMessage(msg, clientID)
	} else {
		msg := fmt.Sprintf("Wrong guess: %v!\n", message)
		if _, err := rw.WriteString(msg); err != nil {
			return err
		}
	}
	return nil
}

// startNewGuessWordGame initializes and starts a new guess word game.
func startNewGuessWordGame(app *app, rw *bufio.ReadWriter) error {
	word, err := words.GenerateWord()
	if err != nil {
		if _, writeErr := rw.WriteString("An error happend, please try again."); writeErr != nil {
			return writeErr
		}

		return err
	}

	app.logger.Info("word chosen", slog.String("word", word))
	app.guessWordGameEngine = app.guessWordGameEngine.NewGame(word)
	return nil
}

// showWelcomeMsg returns the welcome message.
func showWelcomeMsg(app *app, clientID int64) string {
	var welcomeMsg string
	idxInsideWaitingQueue := app.clients.IndexClientInWaitingQueue(clientID)

	if idxInsideWaitingQueue != -1 {
		welcomeMsg = fmt.Sprintf(
			"You are inside the waiting queue, please wait to enter the discussion!\n"+
				"You are in the position %d in the queue.\n\n",
			idxInsideWaitingQueue+1)
	} else {
		welcomeMsg = fmt.Sprintf(
			"Welcome to the server!\n"+
				"You are now connected as client #%d\n"+
				"Number of clients connected: %d\n\n"+
				"Be careful! Messages from the server start with a single `[server]` statement.\n"+
				"Have fun!\n\n"+
				showSpecialCommands(),
			clientID,
			app.clients.Count())
	}

	return welcomeMsg
}

// showSpecialCommands returns the list of commands.
func showSpecialCommands() string {
	commandsMsg := fmt.Sprintf(
		"[server] Special commands:\n"+
			"\t%v \t\t %v\n"+
			"\t%v \t %v\n"+
			"\t%v \t\t %v\n"+
			"\t%v \t %v\n\n",
		commandCountClients,
		commandCountClientsDesc,
		commandSpecialCommands,
		commandSpecialCommandsDesc,
		commandGame, commandGameDesc,
		commandEndGame, commandEndGameDesc)

	return commandsMsg
}
