package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	pkgerr "github.com/pkg/errors"
)

type stackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			logger.Info("Served request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.Host),
			)
		})
	}
}

func initializeLogger(logFile string) (*slog.Logger, closeFunc, error) {
	var closeF closeFunc
	var logger *slog.Logger

	stdErrLogger := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
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

		fileLogger := slog.NewJSONHandler(bufferedFile, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: replaceAttr,
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

func replaceAttr(groups []string, attribute slog.Attr) slog.Attr {
	if attribute.Key == "error" {
		err, ok := attribute.Value.Any().(error)
		if !ok {
			return attribute
		}
		if stackErr, ok := errors.AsType[stackTracer](err); ok {
			return slog.GroupAttrs("error", slog.Attr{
				Key:   "message",
				Value: slog.StringValue(stackErr.Error()),
			}, slog.Attr{
				Key:   "stack_trace",
				Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
			})
		}
	}
	return attribute
}
