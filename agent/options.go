package agent

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Option is a function that configures an agent
type Option func(*agent)

// WithAPIKey sets the API key for the agent
// Takes precedence over TEMPEST_API_KEY environment variable
func WithAPIKey(apiKey string) Option {
	return func(a *agent) {
		a.apiKey = apiKey
	}
}

// WithServerAddr sets the HTTP server address for the agent
// Takes precedence over TEMPEST_SERVER_ADDR environment variable (default: ":8080")
func WithServerAddr(addr string) Option {
	return func(a *agent) {
		a.server.Addr = addr
	}
}

// WithAPIURL sets the base URL for the Tempest API
// Takes precedence over TEMPEST_API_URL environment variable (default: "https://api.tempestdx.com")
func WithAPIURL(url string) Option {
	return func(a *agent) {
		a.apiURL = url
	}
}

// WithLogger sets a custom logger for the agent
// Takes precedence over the logger created from environment variables
func WithLogger(logger *slog.Logger) Option {
	return func(a *agent) {
		a.logger = logger
	}
}

// WithWorkers sets the number of operation worker goroutines
// Takes precedence over TEMPEST_NUM_WORKERS environment variable (default: 5)
func WithWorkers(numWorkers int) Option {
	return func(a *agent) {
		a.workers = numWorkers
	}
}

// WithResultWorkers sets the number of result reporting goroutines
// Takes precedence over TEMPEST_NUM_RESULT_WORKERS environment variable (default: 2)
func WithResultWorkers(numWorkers int) Option {
	return func(a *agent) {
		a.resultWorkers = numWorkers
	}
}

// WithPollTimeout configures the timeout duration for polling operations
// from the queue. A shorter timeout results in more frequent polling but
// can increase CPU usage.
func WithPollTimeout(timeout time.Duration) Option {
	return func(a *agent) {
		a.pollTimeout = timeout
	}
}

// WithLogLevel configures the log level for the agent logger
// Takes precedence over TEMPEST_LOG_LEVEL environment variable (default: "info")
func WithLogLevel(level slog.Level) Option {
	return func(a *agent) {
		// Create a new handler with the specified level
		var handler slog.Handler
		if strings.ToLower(getEnv("TEMPEST_LOG_FORMAT", "json")) == "text" {
			handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
		} else {
			handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
		}

		// Replace the logger
		a.logger = slog.New(handler)
	}
}

// WithStopOptions configures the default options for stopping the agent
// These options determine how the agent shuts down when Stop() is called
func WithStopOptions(options StopOptions) Option {
	return func(a *agent) {
		a.stopOptions = options
	}
}
