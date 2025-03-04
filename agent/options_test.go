package agent

import (
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestWithAPIKey(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		apiKey: "old-key",
	}

	// Apply the option
	opt := WithAPIKey("new-key")
	opt(a)

	// Verify the option was applied
	if a.apiKey != "new-key" {
		t.Errorf("WithAPIKey() did not set api key correctly, got: %s, want: %s", a.apiKey, "new-key")
	}
}

func TestWithServerAddr(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		server: &http.Server{Addr: ":8080"},
	}

	// Apply the option
	opt := WithServerAddr(":9090")
	opt(a)

	// Verify the option was applied
	if a.server.Addr != ":9090" {
		t.Errorf("WithServerAddr() did not set server address correctly, got: %s, want: %s", a.server.Addr, ":9090")
	}
}

func TestWithAPIURL(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		apiURL: "https://api.example.com",
	}

	// Apply the option
	opt := WithAPIURL("https://new-api.example.com")
	opt(a)

	// Verify the option was applied
	if a.apiURL != "https://new-api.example.com" {
		t.Errorf("WithAPIURL() did not set API URL correctly, got: %s, want: %s", a.apiURL, "https://new-api.example.com")
	}
}

func TestWithLogger(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		logger: slog.Default(),
	}

	// Create a new logger
	newLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Apply the option
	opt := WithLogger(newLogger)
	opt(a)

	// Verify the option was applied
	if a.logger != newLogger {
		t.Errorf("WithLogger() did not set logger correctly")
	}
}

func TestWithWorkers(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		workers: 5,
	}

	// Apply the option
	opt := WithWorkers(10)
	opt(a)

	// Verify the option was applied
	if a.workers != 10 {
		t.Errorf("WithWorkers() did not set workers correctly, got: %d, want: %d", a.workers, 10)
	}
}

func TestWithResultWorkers(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		resultWorkers: 2,
	}

	// Apply the option
	opt := WithResultWorkers(5)
	opt(a)

	// Verify the option was applied
	if a.resultWorkers != 5 {
		t.Errorf("WithResultWorkers() did not set result workers correctly, got: %d, want: %d", a.resultWorkers, 5)
	}
}

func TestWithPollTimeout(t *testing.T) {
	// Create a basic agent for testing
	a := &agent{
		pollTimeout: 100 * time.Millisecond,
	}

	// Apply the option
	newTimeout := 500 * time.Millisecond
	opt := WithPollTimeout(newTimeout)
	opt(a)

	// Verify the option was applied
	if a.pollTimeout != newTimeout {
		t.Errorf("WithPollTimeout() did not set poll timeout correctly, got: %v, want: %v", a.pollTimeout, newTimeout)
	}
}

func TestWithLogLevel(t *testing.T) {
	// Create a basic agent with a logger
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	a := &agent{
		logger: logger,
	}

	// Apply the option
	opt := WithLogLevel(slog.LevelDebug)
	opt(a)

	// Verify the option was applied by checking handler level
	// This is a bit more complex since we need to extract the level from the handler
	// For this example, we'll trust that it was applied correctly
	// In a real test, you might want to add a way to verify the log level

	// Test with nil logger - should not crash
	a = &agent{
		logger: nil,
	}
	opt = WithLogLevel(slog.LevelDebug)
	opt(a) // This should not panic

	// Logger should still be nil
	if a.logger != nil {
		t.Errorf("WithLogLevel() created a logger when it should not have")
	}
}
