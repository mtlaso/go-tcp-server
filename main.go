package main

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
)

// app is an instance grouping functionalities of the app.
// Used for dependency injection.
type app struct {
	logger *slog.Logger
}

// errorout prints the error and terminates the program.
func (app *app) errout(msg string, keyvals ...any) {
	app.logger.Error(msg, keyvals...)
	panic(1)
}

func (app *app) handleConnection(conn net.Conn) {
	app.logger.Info("got a connection:", slog.Any("conn", conn.LocalAddr().String()))
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil {
			app.logger.Error("error closing connection", slog.Any("error", closeErr))
		}
	}()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	for {
		// Read until newline or EOF.
		message, err := rw.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				app.logger.Info("client disconnected", slog.Any("IP", conn.RemoteAddr().String()))
			} else {
				app.logger.Error("error reading client message", slog.Any("error", err))
			}
			break
		}

		trimmedMessage := strings.TrimSpace(message)
		app.logger.Info("received message",
			slog.String(conn.RemoteAddr().String(), trimmedMessage))

		_, err = rw.WriteString("hello from server!\r\n")
		if err != nil {
			app.logger.Error("error writing to client", slog.Any("error", err))
			closeErr := conn.Close()
			if closeErr != nil {
				app.logger.Error("error closing connection", slog.Any("error", closeErr))
			}
		}

		// Flush the buffer to ensuire data is sent.
		err = rw.Flush()
		if err != nil {
			app.logger.Error("error flushing data", slog.Any("error", err))
		}
	}
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	app := &app{
		logger: logger,
	}

	ln, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		app.errout("failed to listen", slog.Any("error", err))
	}
	app.logger.Info("tcp server listening to connections...")

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
