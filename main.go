// Package main implements a concurrent TCP chat server with support for
// multiple clients, message broadcasting, and word guessing games.
//
// Basic usage:
//
//	go run main.go --max-connected-clients=10 --listen-addr=localhost:8080
//
// Connect to the server using netcat:
//
//	nc -q -1 localhost 8080
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"example.com/tcp-clients/app"
)

const (
	flagMaxConnectedClients        = "max-connected-clients"
	flagMaxConnectedClientsDefault = 100
	flagMaxConnectedClientsDesc    = "maximum of connected clients at the same time"
	flagListenAddr                 = "listen-addr"
	flagListenAddrDefault          = "localhost:8080"
	flagListenAddrDesc             = "server listen address (ex : listen-addr=localhost:8080)"
)

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

	app := app.NewServer(logger, maxConnectedClients)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigsChan := make(chan os.Signal, 1)
	signal.Notify(sigsChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		app.Logger.Error("failed to listen", slog.Any("error", err))
		panic(err)
	}
	app.Logger.Info("tcp server listening for connections...")

	// Goroutine to listen for system signals.
	go func() {
		<-sigsChan
		app.Logger.Info("Shutting down the server...")
		app.BroadcastServerShutdown()
		cancel()
		if closeErr := ln.Close(); closeErr != nil {
			app.Logger.Error(
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
				app.Logger.Warn("cannot accept connection", slog.Any("error", err))
				continue
			}
		}

		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			app.HandleConnection(ctx, c)
		}(conn)
	}

	wg.Wait()
	app.Logger.Info("server shutdown")
}
