package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"boot.dev/linko/internal/store"
)

type closeFunc func() error

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	httpPort := flag.Int("port", 8899, "port to listen on")
	dataDir := flag.String("data", "./data", "directory to store data")
	flag.Parse()

	status := run(ctx, cancel, *httpPort, *dataDir)
	cancel()
	os.Exit(status)
}

func run(ctx context.Context, cancel context.CancelFunc, httpPort int, dataDir string) int {
	logger, closeF, err := initializeLogger(os.Getenv("LINKO_LOG_FILE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %d", err)
		return 1
	}
	defer func() {
		if err := closeF(); err != nil {
			fmt.Fprintf(os.Stderr, "%d", err)
		}
	}()

	st, err := store.New(dataDir, logger)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to create store: %v", err))
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
		logger.Error(fmt.Sprintf("failed to shutdown server: %v\n", err))
		return 1
	}
	if serverErr != nil {
		logger.Error(fmt.Sprintf("server error: %v\n", serverErr))
		return 1
	}
	logger.Debug("Linko is shutting down")
	return 0
}

func initializeLogger(logFile string) (*slog.Logger, closeFunc, error) {
	var closeF closeFunc
	var logger *slog.Logger

	stdErrLogger := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	if logFile != "" {
		file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load log file")
		}

		bufferedFile := bufio.NewWriterSize(file, 8192)

		closeF = func() error {
			if err := bufferedFile.Flush(); err != nil {
				return fmt.Errorf("failed flush file: %d", err)
			}

			if err = file.Close(); err != nil {
				return fmt.Errorf("failed close file: %d", err)
			}
			return nil
		}

		fileLogger := slog.NewTextHandler(bufferedFile, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger = slog.New(slog.NewMultiHandler(
			stdErrLogger,
			fileLogger,
		))
	} else {
		logger = slog.New(stdErrLogger)
	}

	return logger, closeF, nil
}
