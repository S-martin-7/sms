package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type ctxKey struct{}

// New returns a zerolog.Logger honoring the given level string.
// In dev env it uses a human-friendly console writer; in prod, JSON.
func New(level, env string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var w io.Writer = os.Stdout
	if env != "prod" {
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}
	return zerolog.New(w).Level(lvl).With().Timestamp().Logger()
}

// FromContext returns the logger stored in ctx, or a disabled one.
func FromContext(ctx context.Context) *zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zerolog.Logger); ok {
		return l
	}
	d := zerolog.Nop()
	return &d
}

// WithLogger returns a ctx carrying l.
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}
