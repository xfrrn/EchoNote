package logging

import (
	"log/slog"
	"os"
)

func New(service, environment string, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("environment", environment),
	)
}
