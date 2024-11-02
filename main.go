package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// clients represents the clients connected to the server.
type clients struct {
	clients map[int64]net.Conn
	mu      sync.RWMutex
	// Do NOT! change this field directly!
	id int64
}

// add adds a client to the clients list.
func (c *clients) add(id int64, conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clients[id] = conn
}

// remove removes a client from the clients list.
func (c *clients) remove(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clients, id)
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
	logger  *slog.Logger
	clients *clients
}

// errorout prints the error and terminates the program.
func (app *app) errout(msg string, keyvals ...any) {
	app.logger.Error(msg, keyvals...)
	panic(1)
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
		if closeErr != nil {
			app.logger.Error("error closing connection", slog.Any("error", closeErr))
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Welcome message.
	welcomeMsg := fmt.Sprintf(
		"Welcome to the server!\n"+
			"You are now connected as client #%d\n"+
			"Number of clients connected: %d\n\n",
		clientID,
		app.clients.count())
	_, err := rw.WriteString(welcomeMsg)
	if err != nil {
		app.logger.Error("error writing to client", slog.Any("error", err))
		return
	}

	// Flush to send message directly to the client.
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

		_, err = rw.WriteString("hello from server!\r\n")
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			return
		}

		// Flush the buffer to ensure data is sent.
		err = rw.Flush()
		if err != nil {
			app.logger.Error("error flushing data", slog.Any("error", err))
			return
		}
	}
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	app := &app{
		logger: logger,
		clients: &clients{
			clients: make(map[int64]net.Conn),
		},
	}

	ln, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		app.errout("failed to listen", slog.Any("error", err))
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
