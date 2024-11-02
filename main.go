package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
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

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	fmt.Println("got a connection:", conn.LocalAddr().String())
}
