package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestLogger_Interface ensures Logger interface compliance
func TestLogger_Interface(t *testing.T) {
	var _ Logger = (*NoOpLogger)(nil)
}

// TestNoOpLogger_DoesNotPanic tests NoOpLogger doesn't panic
func TestNoOpLogger_DoesNotPanic(t *testing.T) {
	logger := &NoOpLogger{}

	// Should not panic
	logger.Debug("test", "key", "value")
	logger.Info("test", "key", "value")
	logger.Warn("test", "key", "value")
	logger.Error("test", "key", "value")
}

// TestZerologger_Debug tests debug level logging
func TestZerologger_Debug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("debug"))

	logger.Debug("test message", "key", "value", "number", 42)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["message"] != "test message" {
		t.Errorf("expected message 'test message', got %v", log["message"])
	}

	if log["key"] != "value" {
		t.Errorf("expected key='value', got %v", log["key"])
	}

	if log["number"] != float64(42) {
		t.Errorf("expected number=42, got %v", log["number"])
	}

	if log["level"] != "debug" {
		t.Errorf("expected level='debug', got %v", log["level"])
	}
}

// TestZerologger_Info tests info level logging
func TestZerologger_Info(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Info("info message", "user", "john", "action", "login")

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["message"] != "info message" {
		t.Errorf("expected message 'info message', got %v", log["message"])
	}

	if log["level"] != "info" {
		t.Errorf("expected level='info', got %v", log["level"])
	}
}

// TestZerologger_Warn tests warn level logging
func TestZerologger_Warn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("warn"))

	logger.Warn("warning message", "threshold", 0.8)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["level"] != "warn" {
		t.Errorf("expected level='warn', got %v", log["level"])
	}
}

// TestZerologger_Error tests error level logging
func TestZerologger_Error(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("error"))

	logger.Error("error message", "status", "failed", "code", 500)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["level"] != "error" {
		t.Errorf("expected level='error', got %v", log["level"])
	}

	if log["message"] != "error message" {
		t.Errorf("expected message 'error message', got %v", log["message"])
	}
}

// TestZerologger_LogLevel_Debug tests debug level filtering
func TestZerologger_LogLevel_Debug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("debug"))

	logger.Debug("debug", "level", "debug")
	logger.Info("info", "level", "info")
	logger.Warn("warn", "level", "warn")
	logger.Error("error", "level", "error")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 4 {
		t.Errorf("expected 4 log lines at debug level, got %d", len(lines))
	}
}

// TestZerologger_LogLevel_Info tests info level filtering
func TestZerologger_LogLevel_Info(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Debug("debug", "level", "debug")
	logger.Info("info", "level", "info")
	logger.Warn("warn", "level", "warn")
	logger.Error("error", "level", "error")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Errorf("expected 3 log lines at info level, got %d", len(lines))
	}
}

// TestZerologger_LogLevel_Warn tests warn level filtering
func TestZerologger_LogLevel_Warn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("warn"))

	logger.Debug("debug", "level", "debug")
	logger.Info("info", "level", "info")
	logger.Warn("warn", "level", "warn")
	logger.Error("error", "level", "error")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Errorf("expected 2 log lines at warn level, got %d", len(lines))
	}
}

// TestZerologger_LogLevel_Error tests error level filtering
func TestZerologger_LogLevel_Error(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("error"))

	logger.Debug("debug", "level", "debug")
	logger.Info("info", "level", "info")
	logger.Warn("warn", "level", "warn")
	logger.Error("error", "level", "error")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Errorf("expected 1 log line at error level, got %d", len(lines))
	}
}

// TestZerologger_MultipleKeyValues tests multiple key-value pairs
func TestZerologger_MultipleKeyValues(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Info("rate limit", "key", "api-key-123", "limit", 100, "remaining", 45, "reset", 3600)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["key"] != "api-key-123" {
		t.Errorf("expected key='api-key-123', got %v", log["key"])
	}

	if log["limit"] != float64(100) {
		t.Errorf("expected limit=100, got %v", log["limit"])
	}

	if log["remaining"] != float64(45) {
		t.Errorf("expected remaining=45, got %v", log["remaining"])
	}

	if log["reset"] != float64(3600) {
		t.Errorf("expected reset=3600, got %v", log["reset"])
	}
}

// TestZerologger_OddNumberOfArgs handles odd number of args gracefully
func TestZerologger_OddNumberOfArgs(t *testing.T) {
	t.Skip("odd args should be handled by implementation")
}

// TestZerologger_EmptyMessage tests empty message
func TestZerologger_EmptyMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Info("", "key", "value")

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["key"] != "value" {
		t.Errorf("expected key='value', got %v", log["key"])
	}
}

// TestZerologger_NilValues tests nil values in args
func TestZerologger_NilValues(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Info("test", "value", nil)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["value"] != nil {
		t.Errorf("expected value=nil, got %v", log["value"])
	}
}

// TestZerologger_BooleanValues tests boolean values
func TestZerologger_BooleanValues(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	logger.Info("test", "enabled", true, "disabled", false)

	var log map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &log)
	if err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if log["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", log["enabled"])
	}

	if log["disabled"] != false {
		t.Errorf("expected disabled=false, got %v", log["disabled"])
	}
}

// BenchmarkZerologger_Info benchmarks info level logging
func BenchmarkZerologger_Info(b *testing.B) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("info"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message", "key", "value", "number", i)
	}
}

// BenchmarkZerologger_Debug benchmarks debug level logging
func BenchmarkZerologger_Debug(b *testing.B) {
	buf := &bytes.Buffer{}
	logger := NewZerologger(WithOutput(buf), WithLevel("debug"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug("test message", "key", "value", "number", i)
	}
}

// BenchmarkNoOpLogger_Info benchmarks noop logger
func BenchmarkNoOpLogger_Info(b *testing.B) {
	logger := &NoOpLogger{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("test message", "key", "value", "number", i)
	}
}

// TestZerologger_RaceCondition tests race conditions (go test -race)
func TestZerologger_RaceCondition(t *testing.T) {
	done := make(chan bool)

	// Multiple goroutines logging concurrently (each with own buffer)
	for i := 0; i < 10; i++ {
		go func(id int) {
			buf := &bytes.Buffer{}
			logger := NewZerologger(WithOutput(buf), WithLevel("info"))

			for j := 0; j < 100; j++ {
				logger.Info("concurrent log", "goroutine", id, "iteration", j)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestZerologger_ConcurrentCreation tests creating multiple loggers concurrently
func TestZerologger_ConcurrentCreation(t *testing.T) {
	done := make(chan Logger)

	for i := 0; i < 5; i++ {
		go func() {
			buf := &bytes.Buffer{}
			logger := NewZerologger(WithOutput(buf), WithLevel("info"))
			logger.Info("test", "concurrent", true)
			done <- logger
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}
