package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func initializeLogger(logFile string) (*log.Logger, error) {
	var multiWriter io.Writer
	if logFile == "" {
		multiWriter = io.MultiWriter(os.Stderr)
	} else {
		file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to load log file")
		}

		multiWriter = io.MultiWriter(os.Stderr, file)
	}

	logger := log.New(multiWriter, "", log.LstdFlags)
	return logger, nil
}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {
	logger, err := initializeLogger(os.Getenv("LINKO_LOG_FILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %d", err)
		return 1
	}
	st, err := store.New(dataDir, logger)
	if err != nil {
		logger.Printf("failed to create store: %v", err)
		return 1
	}
	s := newServer(*st, httpPort, cancel, logger)
	var serverErr error
	go func() {
		serverErr = s.start()
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.shutdown(shutdownCtx); err != nil {
		logger.Printf("failed to shutdown server: %v\n", err)
		return 1
	}
	if serverErr != nil {
		logger.Printf("server error: %v\n", serverErr)
		return 1
	}
	logger.Print("Linko is shutting down\n")
	return 0
}
