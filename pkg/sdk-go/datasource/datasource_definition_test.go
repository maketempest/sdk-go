package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tempestdx/sdk-go/jsonschema"
)

func TestNew(t *testing.T) {
	// Valid output schema
	validOutputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object", "properties": {"name": {"type": "string"}}}`),
	}

	// Valid input schema
	validInputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object", "properties": {"id": {"type": "string"}}}`),
	}

	// Valid get function
	validGetFunc := func(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error) {
		return &WebhookResponse{
			Data: map[string]any{"name": "Test Data"},
		}, nil
	}

	tests := []struct {
		name          string
		config        Config
		opts          []Option
		expectedError bool
	}{
		{
			name: "Valid configuration",
			config: Config{
				DisplayName:  "Test Datasource",
				UniqueID:     "test-datasource",
				OutputSchema: validOutputSchema,
				InputSchema:  validInputSchema,
			},
			opts: []Option{
				WithGetFunc(validGetFunc),
				WithInstructions("Test instructions"),
				WithCredentials("api-key"),
			},
			expectedError: false,
		},
		{
			name: "Missing display name",
			config: Config{
				UniqueID:     "test-datasource",
				OutputSchema: validOutputSchema,
			},
			opts:          []Option{WithGetFunc(validGetFunc)},
			expectedError: true,
		},
		{
			name: "Missing unique ID",
			config: Config{
				DisplayName:  "Test Datasource",
				OutputSchema: validOutputSchema,
			},
			opts:          []Option{WithGetFunc(validGetFunc)},
			expectedError: true,
		},
		{
			name: "Missing output schema",
			config: Config{
				DisplayName: "Test Datasource",
				UniqueID:    "test-datasource",
			},
			opts:          []Option{WithGetFunc(validGetFunc)},
			expectedError: true,
		},
		{
			name: "Invalid output schema",
			config: Config{
				DisplayName: "Test Datasource",
				UniqueID:    "test-datasource",
				OutputSchema: &jsonschema.Schema{
					Raw: json.RawMessage(`{invalid json}`),
				},
			},
			opts:          []Option{WithGetFunc(validGetFunc)},
			expectedError: true,
		},
		{
			name: "Invalid input schema",
			config: Config{
				DisplayName:  "Test Datasource",
				UniqueID:     "test-datasource",
				OutputSchema: validOutputSchema,
				InputSchema: &jsonschema.Schema{
					Raw: json.RawMessage(`{invalid json}`),
				},
			},
			opts:          []Option{WithGetFunc(validGetFunc)},
			expectedError: true,
		},
		{
			name: "Missing get function",
			config: Config{
				DisplayName:  "Test Datasource",
				UniqueID:     "test-datasource",
				OutputSchema: validOutputSchema,
			},
			opts:          []Option{},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := New(tt.config, tt.opts...)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, def)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, def)
			}
		})
	}
}

func TestDefinitionMethods(t *testing.T) {
	// Create a valid definition for testing the methods
	outputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object"}`),
	}
	inputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object"}`),
	}
	getFunc := func(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error) {
		return &WebhookResponse{Data: map[string]any{"test": "data"}}, nil
	}

	def, err := New(
		Config{
			DisplayName:  "Test Datasource",
			UniqueID:     "test-datasource",
			OutputSchema: outputSchema,
			InputSchema:  inputSchema,
		},
		WithGetFunc(getFunc),
		WithInstructions("Test instructions"),
		WithCredentials("api-key", "oauth"),
	)
	assert.NoError(t, err)
	assert.NotNil(t, def)

	// Test getter methods
	t.Run("UniqueID", func(t *testing.T) {
		assert.Equal(t, "test-datasource", def.UniqueID())
	})

	t.Run("DisplayName", func(t *testing.T) {
		assert.Equal(t, "Test Datasource", def.DisplayName())
	})

	t.Run("OutputSchema", func(t *testing.T) {
		assert.Equal(t, outputSchema, def.OutputSchema())
	})

	t.Run("InputSchema", func(t *testing.T) {
		assert.Equal(t, inputSchema, def.InputSchema())
	})

	t.Run("InstructionsMarkdown", func(t *testing.T) {
		assert.Equal(t, "Test instructions", def.InstructionsMarkdown())
	})

	t.Run("GetFunc", func(t *testing.T) {
		fn := def.GetFunc()
		assert.NotNil(t, fn)

		// Test the function returns expected result
		resp, err := fn(context.Background(), &WebhookRequest{})
		assert.NoError(t, err)
		assert.Equal(t, map[string]any{"test": "data"}, resp.Data)
	})

	t.Run("CredentialProviders", func(t *testing.T) {
		creds := def.CredentialProviders()
		assert.Equal(t, []string{"api-key", "oauth"}, creds)
	})
}

func TestDefinitionJSON(t *testing.T) {
	// Create a valid definition for testing JSON serialization
	outputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object", "properties": {"name": {"type": "string"}}}`),
	}
	inputSchema := &jsonschema.Schema{
		Raw: json.RawMessage(`{"type": "object", "properties": {"id": {"type": "string"}}}`),
	}

	def, err := New(
		Config{
			DisplayName:  "Test Datasource",
			UniqueID:     "test-datasource",
			OutputSchema: outputSchema,
			InputSchema:  inputSchema,
		},
		WithGetFunc(func(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error) {
			return &WebhookResponse{}, nil
		}),
		WithInstructions("Test instructions"),
		WithCredentials("api-key"),
	)
	assert.NoError(t, err)

	// Test JSON serialization
	jsonBytes, err := def.JSON()
	assert.NoError(t, err)
	assert.NotNil(t, jsonBytes)

	// Deserialize and verify the contents
	var data map[string]any
	err = json.Unmarshal(jsonBytes, &data)
	assert.NoError(t, err)

	assert.Equal(t, "Test Datasource", data["displayName"])
	assert.Equal(t, "test-datasource", data["uniqueID"])
	assert.Equal(t, "Test instructions", data["instructionsMarkdown"])
	assert.Equal(t, []any{"api-key"}, data["credentialProviders"])
	assert.NotNil(t, data["outputSchema"])
	assert.NotNil(t, data["inputSchema"])
}

func TestWebhookResponse(t *testing.T) {
	// Test creating responses with different data
	t.Run("With Data", func(t *testing.T) {
		resp := &WebhookResponse{
			Data: map[string]any{"name": "Test"},
		}
		assert.Equal(t, map[string]any{"name": "Test"}, resp.Data)
		assert.Nil(t, resp.Error)
		assert.Equal(t, "", resp.Message)
	})

	t.Run("With Error", func(t *testing.T) {
		testErr := errors.New("test error")
		resp := &WebhookResponse{
			Error:   testErr,
			Message: "An error occurred",
		}
		assert.Nil(t, resp.Data)
		assert.Equal(t, testErr, resp.Error)
		assert.Equal(t, "An error occurred", resp.Message)
	})
}
