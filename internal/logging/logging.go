package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strings"
)

// Field is a structured logging attribute.
type Field struct {
	Key   string
	Value any
}

// Convenience helpers for common field types.
func String(key, value string) Field  { return Field{Key: key, Value: value} }
func Int(key string, value int) Field { return Field{Key: key, Value: value} }
func Any(key string, value any) Field { return Field{Key: key, Value: value} }

// Logger is a small structured logging interface that can be backed by slog or
// other structured loggers.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Config controls basic logger behaviour.
type Config struct {
	Level     string // debug, info, warn, error
	Format    string // json or text
	AddSource bool   // include source locations
}

// New constructs a Logger backed by slog with the provided config.
func New(cfg Config) Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return &slogger{l: slog.New(handler)}
}

// NewFromEnv constructs a logger using LOG_LEVEL and LOG_FORMAT environment
// variables, defaulting to a human-readable text handler at info level.
func NewFromEnv() Logger {
	level := os.Getenv("LOG_LEVEL")
	format := os.Getenv("LOG_FORMAT")
	return New(Config{
		Level:     level,
		Format:    format,
		AddSource: true,
	})
}

// Noop returns a logger that drops all logs.
func Noop() Logger { return noopLogger{} }

type slogger struct {
	l *slog.Logger
}

func (s *slogger) With(fields ...Field) Logger {
	return &slogger{l: s.l.With(toArgs(fields...)...)}
}

func (s *slogger) Debug(ctx context.Context, msg string, fields ...Field) {
	s.l.LogAttrs(ctx, slog.LevelDebug, msg, toAttrs(fields...)...)
}

func (s *slogger) Info(ctx context.Context, msg string, fields ...Field) {
	s.l.LogAttrs(ctx, slog.LevelInfo, msg, toAttrs(fields...)...)
}

func (s *slogger) Warn(ctx context.Context, msg string, fields ...Field) {
	s.l.LogAttrs(ctx, slog.LevelWarn, msg, toAttrs(fields...)...)
}

func (s *slogger) Error(ctx context.Context, msg string, fields ...Field) {
	s.l.LogAttrs(ctx, slog.LevelError, msg, toAttrs(fields...)...)
}

type noopLogger struct{}

func (noopLogger) With(fields ...Field) Logger             { return noopLogger{} }
func (noopLogger) Debug(context.Context, string, ...Field) {}
func (noopLogger) Info(context.Context, string, ...Field)  {}
func (noopLogger) Warn(context.Context, string, ...Field)  {}
func (noopLogger) Error(context.Context, string, ...Field) {}

func toAttrs(fields ...Field) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields))
	for _, f := range fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	return attrs
}

func toArgs(fields ...Field) []any {
	args := make([]any, 0, len(fields))
	for _, f := range fields {
		args = append(args, slog.Any(f.Key, f.Value))
	}
	return args
}

func parseLevel(level string) slog.Leveler {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ---- Request-scoped helpers ----

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	loggerKey    ctxKey = "logger"
)

// EnsureRequestID attaches a request_id to the context if absent and returns
// the updated context plus the ID.
func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if id := RequestIDFromContext(ctx); id != "" {
		return ctx, id
	}
	id := newRequestID()
	return ContextWithRequestID(ctx, id), id
}

// ContextWithRequestID stores request_id in context.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext extracts request_id from context.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// WithRequestLogger ensures a request_id exists, and returns the updated
// context alongside a logger annotated with that ID.
func WithRequestLogger(ctx context.Context, base Logger) (context.Context, Logger) {
	if base == nil {
		base = Noop()
	}
	ctx, id := EnsureRequestID(ctx)
	return ctx, base.With(String("request_id", id))
}

// ContextWithLogger stores a logger on the context.
func ContextWithLogger(ctx context.Context, l Logger) context.Context {
	if l == nil {
		l = Noop()
	}
	return context.WithValue(ctx, loggerKey, l)
}

// LoggerFromContext fetches a logger from context if present; otherwise it
// returns nil.
func LoggerFromContext(ctx context.Context) Logger {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(loggerKey).(Logger); ok {
		return v
	}
	return nil
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
