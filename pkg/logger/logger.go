// Package logger configures application logging and adapters for HTTP and GORM.
package logger

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/trace"
	"io"
	stdlog "log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Config struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}

// New installs the shared logger before other Wire providers initialize.
// An optional file receives the same records as stderr. Cleanup restores the
// previous logger before closing the file, so startup failures can still log.
func New(cfg Config) (*slog.Logger, func(), error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(cfg.Level))); err != nil {
		return nil, nil, fmt.Errorf("invalid log.level: %w", err)
	}
	if cfg.Format != "text" && cfg.Format != "json" {
		return nil, nil, fmt.Errorf("log.format must be text or json")
	}
	output := io.Writer(os.Stderr)
	var file *os.File
	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0700); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		var err error
		file, err = os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		output = io.MultiWriter(os.Stderr, file)
	}
	log := slog.New(newHandler(output, cfg.Format, level))
	previous := slog.Default()
	previousWriter, previousFlags := stdlog.Writer(), stdlog.Flags()
	slog.SetDefault(log) // Also routes the standard library log package here.
	cleanup := func() {
		slog.SetDefault(previous)
		stdlog.SetOutput(previousWriter)
		stdlog.SetFlags(previousFlags)
		if file != nil {
			_ = file.Close()
		}
	}
	return log, cleanup, nil
}

func newHandler(output io.Writer, format string, level slog.Level) slog.Handler {
	options := &slog.HandlerOptions{Level: level, AddSource: true}
	if format == "json" {
		return slog.NewJSONHandler(output, options)
	}
	return slog.NewTextHandler(output, options)
}

type traceIDKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceID returns the active span trace ID or the request correlation fallback.
func TraceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

func Debug(message string, args ...any) {
	write(context.Background(), slog.LevelDebug, message, args...)
}
func Info(message string, args ...any) { write(context.Background(), slog.LevelInfo, message, args...) }
func Warn(message string, args ...any) { write(context.Background(), slog.LevelWarn, message, args...) }
func Error(message string, args ...any) {
	write(context.Background(), slog.LevelError, message, args...)
}

// LogContext includes the request trace ID when the context comes from HTTP middleware.
func LogContext(ctx context.Context, level slog.Level, message string, args ...any) {
	write(ctx, level, message, args...)
}

func write(ctx context.Context, level slog.Level, message string, args ...any) {
	log := slog.Default()
	if !log.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	// Skip runtime.Callers, write and the public logging wrapper.
	runtime.Callers(3, pcs[:])
	record := slog.NewRecord(time.Now(), level, message, pcs[0])
	record.Add(args...)
	if id := TraceID(ctx); id != "" {
		record.Add("trace_id", id)
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			record.Add("span_id", sc.SpanID().String())
		}
	}
	_ = log.Handler().Handle(ctx, record)
}
