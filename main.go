package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

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
	commandUnknowError             = "unknown command"
	maxLenMsg                      = 200
	flagMaxConnectedClients        = "max-connected-clients"
	flagMaxConnectedClientsDefault = 100
	flagMaxConnectedClientsDesc    = "maximum of connected clients at the same time"
)

// guessWordGameEngine represents the game data when playing the guess word game.
type guessWordGameEngine struct {
	word    string
	mu      sync.RWMutex
	tries   int
	started bool
}

// newGame initialises a new game engine.
func (ge *guessWordGameEngine) newGame(word string) *guessWordGameEngine {
	ge.mu.Lock()
	defer ge.mu.Unlock()
	return &guessWordGameEngine{
		word:    word,
		tries:   0,
		started: true,
	}
}

// checkWord checks if the word was found.
// If found, it returns true and ends the game.
func (ge *guessWordGameEngine) checkWord(word string) bool {
	ge.mu.RLock()
	defer func() {
		ge.tries++
		ge.mu.RUnlock()
	}()

	if !ge.started {
		return false
	}
	if word == ge.word {
		ge.started = false
		return true
	}

	return false
}

// endGame end a game that started.
//
// Return true if the game just ended.
func (ge *guessWordGameEngine) endGame() bool {
	ge.mu.Lock()
	defer ge.mu.Unlock()
	if ge.started {
		ge.started = false
		return true
	}

	return false
}

// clients represents the clients connected to the server.
type clients struct {
	clients             map[int64]net.Conn
	waitingQueueIDs     []int64
	mu                  sync.RWMutex
	maxConnectedClients int
	// Do NOT! change this field directly!
	id int64
}

// addToWaitingQueue adds a client to the waiting queue.
func (c *clients) addToWaitingQueue(clientID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waitingQueueIDs = append(c.waitingQueueIDs, clientID)
}

// updateWaitingQueue removes a client from the waiting queue, either by its clientID
// if the client that just left was inside the waiting queue,
// or removes the next client in the waiting queue (first index in slice).
//
// If clientID is -1, it will remove the next client from the waiting queue.
func (c *clients) updateWaitingQueue(clientID int64) {
	idx := c.indexClientInWaitingQueue(clientID)

	c.mu.Lock()
	if len(c.waitingQueueIDs) != 0 {
		if idx == -1 {
			_, c.waitingQueueIDs = c.waitingQueueIDs[0], c.waitingQueueIDs[1:]
		} else {
			c.waitingQueueIDs = slices.DeleteFunc(c.waitingQueueIDs, func(el int64) bool {
				return el == clientID
			})
		}
	}
	c.mu.Unlock()
}

// indexClientInWaitingQueue returns the index of the client, by it's clientID, if it's inside the waiting queue.
// If the clientID is not inside the waiting queue, it returns -1.
func (c *clients) indexClientInWaitingQueue(clientID int64) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Index(c.waitingQueueIDs, clientID)
}

// add adds a client to the clients list.
func (c *clients) add(id int64, conn net.Conn) {
	c.mu.Lock()
	c.clients[id] = conn
	c.mu.Unlock() // Unlock now to avoid deadlock inside addToWaitingQueue().

	if len(c.clients) > c.maxConnectedClients {
		c.addToWaitingQueue(id)
	}
}

// remove removes a client from the clients list and updates the waiting queue.
func (c *clients) remove(clientID int64) {
	c.mu.Lock()
	delete(c.clients, clientID)
	c.mu.Unlock() // Unlock now to prevent deadlock.

	c.updateWaitingQueue(clientID)
}

// count returns the number of clients connected to the server.
func (c *clients) count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.clients)
}

// nextID returns the next id of a client.
func (c *clients) nextID() int64 {
	return atomic.AddInt64(&c.id, 1)
}

// app is an instance grouping functionalities of the app.
// Used for dependency injection.
type app struct {
	logger              *slog.Logger
	clients             *clients
	guessWordGameEngine *guessWordGameEngine
}

// broadcastMessage broadcasts a message to all other clients except the one who sent the broadcastMessage
// and the ones who are inside the waiting queue.
//
// clientID : client ID of the client who sent the message.
func (app *app) broadcastMessage(message string, clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.clients.mu.RLock()
	for k, v := range app.clients.clients {
		if k != clientID && !slices.Contains(app.clients.waitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.clients.mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[%v][client #%v] %v\n", client.RemoteAddr().String(), clientID, message)
		_, err := client.Write([]byte(msg))
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	}
}

// broadcastServerMessageClientLeft broadcasts an official server message to all
// other clients and clients not inside the waiting queue that the client with clientID left.
//
// clientID : client ID of the client who left.
func (app *app) broadcastServerMessageClientLeft(clientID int64) {
	otherClients := make(map[int64]net.Conn)

	app.clients.mu.RLock()
	for k, v := range app.clients.clients {
		if k != clientID && !slices.Contains(app.clients.waitingQueueIDs, k) {
			otherClients[k] = v
		}
	}
	app.clients.mu.RUnlock()

	for _, client := range otherClients {
		// clientID here is the client_id of the client who sent the message!
		msg := fmt.Sprintf("[server] client #%v left the server.\n\n", clientID)
		_, err := client.Write([]byte(msg))
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	}
}

// broadcastGameStarted broadcasts that a game started.
func (app *app) broadcastGameStarted() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.clients.mu.RLock()
	for k, v := range app.clients.clients {
		if !slices.Contains(app.clients.waitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.clients.mu.RUnlock()

	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game started! Try to guess to word\n"+
			"The word has %d letters.\n"+
			"The word starts with: %v\n\n",
			len(app.guessWordGameEngine.word),
			string(app.guessWordGameEngine.word[0]))
		_, err := client.Write([]byte(msg))
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	}
}

// broadcastGameEnded broadcasts that the game ended.
func (app *app) broadcastGameEnded() {
	clientsThatWillPlay := make(map[int64]net.Conn)

	app.clients.mu.RLock()
	for k, v := range app.clients.clients {
		if !slices.Contains(app.clients.waitingQueueIDs, k) {
			clientsThatWillPlay[k] = v
		}
	}
	app.clients.mu.RUnlock()

	for _, client := range clientsThatWillPlay {
		msg := fmt.Sprintf("Guess a word game ended! The word was : %v\n"+
			"Have a good day!\n\n",
			app.guessWordGameEngine.word)
		_, err := client.Write([]byte(msg))
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	}
}

// broadcastQueueStatusToClientsWaiting broadcasts the status of the waiting queue
// to the clients who are in the waiting queue.
func (app *app) broadcastQueueStatusToClientsWaiting() {
	clientsInWaitingQueue := make(map[int64]net.Conn)

	app.clients.mu.RLock()
	for k, v := range app.clients.clients {
		idx := slices.Index(app.clients.waitingQueueIDs, k)
		if idx != -1 {
			clientsInWaitingQueue[int64(idx)] = v
		}
	}
	app.clients.mu.RUnlock()

	for k, client := range clientsInWaitingQueue {
		msg := fmt.Sprintf("You are in the position %d in the queue.\n\n", k+1)
		_, err := client.Write([]byte(msg))
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	}
}

// handleConnection handles a connection to the server.
func (app *app) handleConnection(conn net.Conn) {
	clientID := app.clients.nextID()

	app.clients.add(clientID, conn)
	app.logger.Info("got a connection:",
		slog.Any("client_id", clientID),
		slog.Any("remote_addr", conn.RemoteAddr().String()))

	defer func() {
		closeErr := conn.Close()
		app.clients.remove(clientID)
		app.broadcastServerMessageClientLeft(clientID)
		app.broadcastQueueStatusToClientsWaiting()
		if closeErr != nil {
			app.logger.Error("error closing connection", slog.Any("error", closeErr))
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Welcome message.
	_, err := rw.WriteString(showWelcomeMsg(app, clientID))
	if err != nil {
		app.logger.Error("error writing to client", slog.Any("error", err))
		return
	}

	// Flush to send the welcome message directly to the client.
	err = rw.Flush()
	if err != nil {
		app.logger.Error("error flushing data", slog.Any("error", err))
		return
	}

	for {
		var message string

		// Read until newline or EOF.
		message, err = rw.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				app.logger.Info("client disconnected",
					slog.Any("client_id", clientID),
					slog.Any("remote_addr", conn.RemoteAddr().String()))
			} else {
				app.logger.Error("error reading client message",
					slog.Any("client_id", clientID),
					slog.Any("error", err))
			}
			return
		}

		trimmedMessage := strings.TrimSpace(message)
		app.logger.Info("received message",
			slog.String("remote_addr", conn.RemoteAddr().String()),
			slog.Any("client_id", clientID),
			slog.String("message", trimmedMessage))

		handleMessage(trimmedMessage, app, rw, clientID)

		// Flush the buffer to ensure data is sent.
		err = rw.Flush()
		if err != nil {
			app.logger.Error("error flushing data", slog.Any("error", err))
			return
		}
	}
}

func main() {
	maxConnectedClients := flag.Int(
		flagMaxConnectedClients,
		flagMaxConnectedClientsDefault,
		flagMaxConnectedClientsDesc)

	flag.Parse()

	if *maxConnectedClients <= 1 {
		panic("cannot have 1 or less as max-connected-clients")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	app := &app{
		logger: logger,
		clients: &clients{
			clients:             make(map[int64]net.Conn),
			maxConnectedClients: *maxConnectedClients,
			waitingQueueIDs:     make([]int64, 0, *maxConnectedClients),
		},
		guessWordGameEngine: &guessWordGameEngine{
			word:    "",
			tries:   0,
			started: false,
		},
	}

	ln, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		app.logger.Error("failed to listen", slog.Any("error", err))
		panic(err)
	}
	app.logger.Info("tcp server listening for connections...")

	for {
		var conn net.Conn

		conn, err = ln.Accept()
		if err != nil {
			app.logger.Warn("cannot accept connection", slog.Any("error", err))
			continue
		}

		go app.handleConnection(conn)
	}
}

// handleMessage handles the message send to the server accordingly.
func handleMessage(message string, app *app, rw *bufio.ReadWriter, clientID int64) {
	idxInsideWaitingQueue := app.clients.indexClientInWaitingQueue(clientID)

	switch {
	case len(message) == 0 || len(message) > maxLenMsg:
		return
	case idxInsideWaitingQueue != -1:
		return
	case message == commandCountClients:
		count := fmt.Sprintf("[server] %d\n\n", app.clients.count())
		_, err := rw.WriteString(count)
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandSpecialCommands:
		_, err := rw.WriteString(showSpecialCommands())
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}
	case message == commandGame:
		if !app.guessWordGameEngine.started {
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
		if app.guessWordGameEngine.endGame() {
			app.broadcastGameEnded()
		}

	case message[0] == '/':
		msg := fmt.Sprintf("[server] %v '%v'\n\n", commandUnknowError, message)
		_, err := rw.WriteString(msg)
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}

	default:
		if !app.guessWordGameEngine.started {
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
	wordFound := app.guessWordGameEngine.checkWord(message)
	if wordFound {
		msg := fmt.Sprintf("Congratulations! You guessed the word '%s' correctly.\n", app.guessWordGameEngine.word)
		if _, err := rw.WriteString(msg); err != nil {
			return err
		}

		app.guessWordGameEngine.endGame()

		msg = fmt.Sprintf("Client #%d guessed the word '%s' correctly!", clientID, app.guessWordGameEngine.word)
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

	app.guessWordGameEngine = app.guessWordGameEngine.newGame(word)
	return nil
}

// showWelcomeMsg returns the welcome message.
func showWelcomeMsg(app *app, clientID int64) string {
	var welcomeMsg string
	idxInsideWaitingQueue := app.clients.indexClientInWaitingQueue(clientID)

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
			app.clients.count())
	}

	return welcomeMsg
}

// showSpecialCommands returns the list of commands.
func showSpecialCommands() string {
	commandsMsg := fmt.Sprintf(
		"Special commands:\n"+
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
