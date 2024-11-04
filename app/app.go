// Package app manages the app logic.
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"example.com/tcp-clients/client"
	"example.com/tcp-clients/guessword"
	"example.com/tcp-clients/words"
)

const (
	commandCountClients        = "/count"
	commandCountClientsDesc    = "number of connected clients to the server"
	commandSpecialCommands     = "/commands"
	commandSpecialCommandsDesc = "show special commands"
	commandGame                = "/game"
	commandGameDesc            = "play guess a word with other users (english words)"
	commandEndGame             = "/endgame"
	commandEndGameDesc         = "end game session"
	maxLenMsg                  = 200
)

// App is an instance grouping functionalities of the App.
// Used for dependency injection.
type App struct {
	Logger              *slog.Logger
	Clients             *client.Clients
	GuessWordGameEngine *guessword.GameEngine
}

// NewServer returns a new server with all fields initialized.
func NewServer(logger *slog.Logger, maxConnectedClients *int) *App {
	return &App{
		Logger: logger,
		Clients: &client.Clients{
			Clients:             make(map[int64]net.Conn),
			MaxConnectedClients: *maxConnectedClients,
			WaitingQueueIDs:     make([]int64, 0, *maxConnectedClients),
		},
		GuessWordGameEngine: &guessword.GameEngine{
			Word:    "",
			Tries:   0,
			Started: false,
		},
	}
}

// broadcastMessage broadcasts a message to all other clients except the one who sent the broadcastMessage
// and the ones who are inside the waiting queue.
//
// clientID : client ID of the client who sent the message.
func (app *App) broadcastMessage(message string, clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.Clients.Mu.RLock()
	for k, v := range app.Clients.Clients {
		if k != clientID && !slices.Contains(app.Clients.WaitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.Clients.Mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[%v][client #%v] %v\n", client.RemoteAddr().String(), clientID, message)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastServerMessageClientLeft broadcasts an official server message to all
// other clients and clients not inside the waiting queue that the client with clientID left.
//
// clientID : client ID of the client who left.
func (app *App) broadcastServerMessageClientLeft(clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.Clients.Mu.RLock()
	for k, v := range app.Clients.Clients {
		if k != clientID && !slices.Contains(app.Clients.WaitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.Clients.Mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[server] client #%v left the server.\n\n", clientID)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastGameStarted broadcasts that a game started.
func (app *App) broadcastGameStarted() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.Clients.Mu.RLock()
	for k, v := range app.Clients.Clients {
		if !slices.Contains(app.Clients.WaitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.Clients.Mu.RUnlock()

	app.GuessWordGameEngine.Mu.RLock()
	defer app.GuessWordGameEngine.Mu.RUnlock()
	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game started! Try to guess to word\n"+
			"The word has %d letters.\n"+
			"The word starts with: %v\n\n",
			len(app.GuessWordGameEngine.Word),
			string(app.GuessWordGameEngine.Word[0]))
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastGameEnded broadcasts that the game ended.
func (app *App) broadcastGameEnded() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.Clients.Mu.RLock()
	for k, v := range app.Clients.Clients {
		if !slices.Contains(app.Clients.WaitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.Clients.Mu.RUnlock()

	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game ended! The word was : %v\n"+
			"Have a good day!\n\n",
			app.GuessWordGameEngine.Word)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastQueueStatusToClientsWaiting broadcasts the status of the waiting queue
// to the clients who are in the waiting queue.
func (app *App) broadcastQueueStatusToClientsWaiting() {
	clientsInWaitingQueue := make(map[int64]net.Conn)

	app.Clients.Mu.RLock()
	for k, v := range app.Clients.Clients {
		idx := slices.Index(app.Clients.WaitingQueueIDs, k)
		if idx != -1 {
			clientsInWaitingQueue[int64(idx)] = v
		}
	}
	app.Clients.Mu.RUnlock()

	for k, client := range clientsInWaitingQueue {
		msg := fmt.Sprintf("You are in the position %d in the queue.\n\n", k+1)
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
		}
	}
}

// broadcastServerShutdown broadcast to the clients that the server is shutting down.
func (app *App) BroadcastServerShutdown() {
	app.Clients.Mu.RLock()
	defer app.Clients.Mu.RUnlock()
	for _, client := range app.Clients.Clients {
		msg := "[server] the server is shutting down. Goodbye!\n"
		if _, err := client.Write([]byte(msg)); err != nil {
			app.Logger.Error("error writing shutdown message to client", slog.Any("error", err))
		}
	}
}

// handleConnection handles a connection to the server.
func (app *App) HandleConnection(ctx context.Context, conn net.Conn) {
	clientID := app.Clients.NextID()

	app.Clients.Add(clientID, conn)
	app.Logger.InfoContext(ctx, "got a connection:",
		slog.Any("client_id", clientID),
		slog.Any("remote_addr", conn.RemoteAddr().String()))

	defer func() {
		closeErr := conn.Close()
		app.Clients.Remove(clientID)
		app.broadcastServerMessageClientLeft(clientID)
		app.broadcastQueueStatusToClientsWaiting()
		if closeErr != nil {
			app.Logger.Error("error closing connection", slog.Any("error", closeErr))
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	_, err := rw.WriteString(showWelcomeMsg(app, clientID))
	if err != nil {
		app.Logger.ErrorContext(ctx, "error writing to client", slog.Any("error", err))
		return
	}

	// Flush to send the welcome message directly to the client.
	err = rw.Flush()
	if err != nil {
		app.Logger.ErrorContext(ctx, "error flushing data", slog.Any("error", err))
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if deadlineErr := conn.SetDeadline(time.Now().Add(time.Second * 1)); deadlineErr != nil {
				app.Logger.ErrorContext(ctx, "error setting read deadline", slog.Any("error", deadlineErr))
				return
			}
			var message string

			message, err = rw.ReadString('\n')
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					continue
				}

				if errors.Is(err, io.EOF) {
					app.Logger.InfoContext(ctx, "client disconnected",
						slog.Any("client_id", clientID),
						slog.Any("remote_addr", conn.RemoteAddr().String()))
				} else {
					app.Logger.ErrorContext(ctx, "error reading client message",
						slog.Any("client_id", clientID),
						slog.Any("error", err))
				}
				return
			}

			trimmedMessage := strings.TrimSpace(message)
			app.Logger.InfoContext(ctx, "received message",
				slog.String("remote_addr", conn.RemoteAddr().String()),
				slog.Any("client_id", clientID),
				slog.String("message", trimmedMessage))

			handleMessage(trimmedMessage, app, rw, clientID)

			// Flush the buffer to ensure data is sent.
			err = rw.Flush()
			if err != nil {
				app.Logger.ErrorContext(ctx, "error flushing data", slog.Any("error", err))
				return
			}
		}
	}
}

// handleMessage handles the message send to the server accordingly.
func handleMessage(message string, app *App, rw *bufio.ReadWriter, clientID int64) {
	idxInsideWaitingQueue := app.Clients.IndexClientInWaitingQueue(clientID)

	switch {
	case len(message) == 0 || len(message) > maxLenMsg:
		msg := fmt.Sprintf("[server] Message length must be between 1 and %d characters.\n", maxLenMsg)
		if _, err := rw.WriteString(msg); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
			return
		}
		return
	case idxInsideWaitingQueue != -1:
		return
	case message == commandCountClients:
		count := fmt.Sprintf("[server] %d\n\n", app.Clients.Count())
		if _, err := rw.WriteString(count); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandSpecialCommands:
		if _, err := rw.WriteString(showSpecialCommands()); err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandGame:
		if !app.GuessWordGameEngine.Started {
			err := startNewGuessWordGame(app, rw)
			if err != nil {
				app.Logger.Error("Error while trying to initialize the guess word game", slog.Any("error", err))
				return
			}

			app.broadcastGameStarted()
		} else {
			_, err := rw.WriteString("A game already started!\n\n")
			if err != nil {
				app.Logger.Error("error writing to client", slog.Any("error", err))
				return
			}
		}

	case message == commandEndGame:
		if app.GuessWordGameEngine.EndGame() {
			app.broadcastGameEnded()
		}

	case message[0] == '/':
		msg := fmt.Sprintf("[server] unknown command '%v'\n\n", message)
		_, err := rw.WriteString(msg)
		if err != nil {
			app.Logger.Error("error writing to client", slog.Any("error", err))
			return
		}

	default:
		if !app.GuessWordGameEngine.Started {
			app.broadcastMessage(message, clientID)
			return
		}

		err := handleGuessWordGame(app, message, rw, clientID)
		if err != nil {
			app.Logger.Error("Error handling the guess a word game", slog.Any("error", err))
			return
		}
	}
}

// handleGuessWordGame handles the logic of the guess word game.
func handleGuessWordGame(app *App, message string, rw *bufio.ReadWriter, clientID int64) error {
	wordFound := app.GuessWordGameEngine.CheckWord(message)
	if wordFound {
		msg := fmt.Sprintf("Congratulations! You guessed the word '%s' correctly.\n", app.GuessWordGameEngine.Word)
		if _, err := rw.WriteString(msg); err != nil {
			return err
		}

		app.GuessWordGameEngine.EndGame()

		msg = fmt.Sprintf("Client #%d guessed the word '%s' correctly!", clientID, app.GuessWordGameEngine.Word)
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
func startNewGuessWordGame(app *App, rw *bufio.ReadWriter) error {
	word, err := words.GenerateWord()
	if err != nil {
		if _, writeErr := rw.WriteString("An error happend, please try again."); writeErr != nil {
			return writeErr
		}

		return err
	}

	app.Logger.Info("word chosen", slog.String("word", word))
	app.GuessWordGameEngine = app.GuessWordGameEngine.NewGame(word)
	return nil
}

// showWelcomeMsg returns the welcome message.
func showWelcomeMsg(app *App, clientID int64) string {
	var welcomeMsg string
	idxInsideWaitingQueue := app.Clients.IndexClientInWaitingQueue(clientID)

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
			app.Clients.Count())
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
