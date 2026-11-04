package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func capture(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := slog.Default()
	previousWriter, previousFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(newHandler(&output, "json", level)))
	t.Cleanup(func() { slog.SetDefault(previous); log.SetOutput(previousWriter); log.SetFlags(previousFlags) })
	return &output
}

func TestLevelFilteringAndStructuredFields(t *testing.T) {
	output := capture(t, slog.LevelWarn)
	Debug("hidden debug")
	Info("hidden info")
	Warn("retry", "task_id", 42)
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != "WARN" || record["msg"] != "retry" || record["task_id"] != float64(42) {
		t.Fatalf("unexpected record: %v", record)
	}
}

func TestFileLoggingAndStandardLogBridge(t *testing.T) {
	cfg := Config{Level: "info", Format: "text"}
	cfg.Level, cfg.Format = "info", "json"
	cfg.File = filepath.Join(t.TempDir(), "logs", "app.log")
	previous := slog.Default()
	for i := 0; i < 2; i++ {
		_, cleanup, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		Info("application", "run", i)
		log.Print("legacy message")
		cleanup()
	}
	if slog.Default() != previous {
		t.Fatal("cleanup did not restore previous logger")
	}
	data, err := os.ReadFile(cfg.File)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("file was not appended: %s", data)
	}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["level"] != "INFO" {
			t.Fatalf("unexpected level: %v", record)
		}
	}
}

func TestInvalidLoggerDoesNotReplaceDefault(t *testing.T) {
	cfg := Config{Level: "info", Format: "text"}
	previous := slog.Default()
	cfg.File = t.TempDir() // A directory cannot be opened as a log file.
	if _, cleanup, err := New(cfg); err == nil || cleanup != nil {
		t.Fatal("expected file error")
	}
	if slog.Default() != previous {
		t.Fatal("failed setup replaced default logger")
	}
}

func TestCallerAndTraceContext(t *testing.T) {
	output := capture(t, slog.LevelInfo)
	_, file, line, _ := runtime.Caller(0)
	Info("caller")
	var record struct {
		Source struct {
			File string
			Line int
		}
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Source.File != file || record.Source.Line != line+1 {
		t.Fatalf("wrong caller: %+v", record.Source)
	}
	output.Reset()
	LogContext(WithTraceID(context.Background(), "request-123"), slog.LevelInfo, "business event")
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.TraceID != "request-123" {
		t.Fatalf("missing trace: %+v", record)
	}
}
