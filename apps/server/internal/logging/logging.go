package logging

import (
	"context"
	"log/slog"
	"os"
)

type loggerContextKey struct{}

func New(service, environment string, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		slog.String("service", service),
		slog.String("environment", environment),
	)
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return fallback
}

func WithAttributes(ctx context.Context, attributes ...any) context.Context {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
		return WithLogger(ctx, logger.With(attributes...))
	}
	return ctx
}
