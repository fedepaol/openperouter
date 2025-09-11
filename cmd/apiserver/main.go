package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/openperouter/openperouter/api/grpc"
	"github.com/openperouter/openperouter/internal/apiserver"
	"github.com/openperouter/openperouter/internal/logging"
)

func main() {
	var (
		logLevel   = flag.String("loglevel", "info", "Log level (debug, info, warn, error)")
		socketPath = flag.String("socket", "/tmp/openperouter.sock", "Path to Unix socket")
	)
	flag.Parse()

	// Set up logging using the internal logging package
	_, err := logging.New(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	slog.Info("starting OpenPERouter API server", "socket", *socketPath)

	// Remove existing socket file if it exists
	if err := os.RemoveAll(*socketPath); err != nil {
		slog.Error("failed to remove existing socket", "error", err)
		os.Exit(1)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen on Unix socket", "socket", *socketPath, "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Set socket permissions (readable/writable by owner and group)
	if err := os.Chmod(*socketPath, 0660); err != nil {
		slog.Warn("failed to set socket permissions", "error", err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	apiServer := apiserver.New()
	pb.RegisterOpenPERouterServiceServer(grpcServer, apiServer)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("received signal, shutting down gracefully", "signal", sig)
		grpcServer.GracefulStop()
		os.RemoveAll(*socketPath)
		os.Exit(0)
	}()

	slog.Info("gRPC server listening", "socket", *socketPath)
	if err := grpcServer.Serve(listener); err != nil {
		slog.Error("failed to serve gRPC", "error", err)
		os.Exit(1)
	}
}

