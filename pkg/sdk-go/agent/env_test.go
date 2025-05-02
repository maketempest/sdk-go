package agent

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	// Test when environment variable exists
	const testKey = "TEST_ENV_VAR"
	const testValue = "test-value"
	const defaultValue = "default-value"

	// Set environment variable
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	result := getEnv(testKey, defaultValue)
	if result != testValue {
		t.Errorf("getEnv(%s, %s) = %s; want %s", testKey, defaultValue, result, testValue)
	}

	// Test when environment variable doesn't exist
	const nonExistentKey = "NON_EXISTENT_KEY"
	result = getEnv(nonExistentKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnv(%s, %s) = %s; want %s", nonExistentKey, defaultValue, result, defaultValue)
	}
}

func TestGetEnvInt(t *testing.T) {
	// Test when environment variable exists and is a valid integer
	const testKey = "TEST_INT_VAR"
	const testValue = "42"
	const defaultValue = 10

	// Set environment variable
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	result := getEnvInt(testKey, defaultValue)
	if result != 42 {
		t.Errorf("getEnvInt(%s, %d) = %d; want %d", testKey, defaultValue, result, 42)
	}

	// Test when environment variable doesn't exist
	const nonExistentKey = "NON_EXISTENT_INT_KEY"
	result = getEnvInt(nonExistentKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvInt(%s, %d) = %d; want %d", nonExistentKey, defaultValue, result, defaultValue)
	}

	// Test when environment variable exists but is not a valid integer
	const invalidKey = "INVALID_INT_KEY"
	os.Setenv(invalidKey, "not-an-int")
	defer os.Unsetenv(invalidKey)

	result = getEnvInt(invalidKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvInt(%s, %d) = %d; want %d", invalidKey, defaultValue, result, defaultValue)
	}
}

func TestGetEnvDuration(t *testing.T) {
	// Test when environment variable exists and is a valid duration
	const testKey = "TEST_DURATION_VAR"
	const testValue = "5s"
	const defaultValue = 10 * time.Second

	// Set environment variable
	os.Setenv(testKey, testValue)
	defer os.Unsetenv(testKey)

	result := getEnvDuration(testKey, defaultValue)
	if result != 5*time.Second {
		t.Errorf("getEnvDuration(%s, %v) = %v; want %v", testKey, defaultValue, result, 5*time.Second)
	}

	// Test when environment variable doesn't exist
	const nonExistentKey = "NON_EXISTENT_DURATION_KEY"
	result = getEnvDuration(nonExistentKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvDuration(%s, %v) = %v; want %v", nonExistentKey, defaultValue, result, defaultValue)
	}

	// Test when environment variable exists but is not a valid duration
	const invalidKey = "INVALID_DURATION_KEY"
	os.Setenv(invalidKey, "not-a-duration")
	defer os.Unsetenv(invalidKey)

	result = getEnvDuration(invalidKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvDuration(%s, %v) = %v; want %v", invalidKey, defaultValue, result, defaultValue)
	}

	// Test with various duration formats
	durationTests := []struct {
		input    string
		expected time.Duration
	}{
		{"1m", time.Minute},
		{"1h30m", 90 * time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"1.5s", 1500 * time.Millisecond},
	}

	for _, tc := range durationTests {
		os.Setenv(testKey, tc.input)
		result = getEnvDuration(testKey, defaultValue)
		if result != tc.expected {
			t.Errorf("getEnvDuration(%s, %v) with value %s = %v; want %v",
				testKey, defaultValue, tc.input, result, tc.expected)
		}
	}
}

func TestGetEnvLogLevel(t *testing.T) {
	// Test when environment variable exists with valid log levels
	const testKey = "TEST_LOG_LEVEL_VAR"
	const defaultValue = slog.LevelInfo

	// Test cases for different log level strings
	logLevelTests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug}, // Test case insensitivity
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn}, // Test alternate name
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"invalid", defaultValue}, // Should return default for invalid level
	}

	for _, tc := range logLevelTests {
		os.Setenv(testKey, tc.input)
		result := getEnvLogLevel(testKey, defaultValue)
		if result != tc.expected {
			t.Errorf("getEnvLogLevel(%s, %v) with value %s = %v; want %v",
				testKey, defaultValue, tc.input, result, tc.expected)
		}
	}

	// Test when environment variable doesn't exist
	os.Unsetenv(testKey)
	result := getEnvLogLevel(testKey, defaultValue)
	if result != defaultValue {
		t.Errorf("getEnvLogLevel(%s, %v) = %v; want %v", testKey, defaultValue, result, defaultValue)
	}
}
