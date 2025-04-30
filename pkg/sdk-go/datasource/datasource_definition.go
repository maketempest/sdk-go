package datasource

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tempestdx/sdk-go/credential"
	"github.com/tempestdx/sdk-go/jsonschema"
)

// Definition represents a datasource definition
type Definition struct {
	// The display name of your datasource
	displayName string
	// The unique ID of your datasource, if not specified, your display name will be slugified
	uniqueID string

	// Schema that defines the output of the datasource
	outputSchema *jsonschema.Schema
	// Schema that defines the input parameters for the datasource
	inputSchema *jsonschema.Schema
	// Documentation for the datasource
	instructionsMarkdown string
	// The function that will be called to get data
	getFunc GetFunc
	// Names of credential providers required by this datasource
	credentialProviders []string
}

// WebhookRequest contains the input parameters for a datasource webhook
type WebhookRequest struct {
	// Input contains the input data for the request, validated against the schema
	Args map[string]any `json:"args,omitempty"`
	// Credentials contains the credentials for the webhook
	Credentials *credential.Credential `json:"credentials,omitempty"`
}

// WebhookResponse contains the output data from a datasource
type WebhookResponse struct {
	// Data is the resulting data from the datasource
	Data any `json:"data,omitempty"`
	// Error is any error that occurred during processing
	Error error `json:"error,omitempty"`
	// Message provides additional context
	Message string `json:"message,omitempty"`
}

// GetFunc is the function signature for datasource retrieval
type GetFunc func(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error)

// Config represents the configuration for creating a new datasource
type Config struct {
	DisplayName  string
	UniqueID     string
	OutputSchema *jsonschema.Schema
	InputSchema  *jsonschema.Schema
}

// New creates a new datasource definition
func New(config Config, opts ...Option) (*Definition, error) {
	d := &Definition{
		displayName:  config.DisplayName,
		uniqueID:     config.UniqueID,
		outputSchema: config.OutputSchema,
		inputSchema:  config.InputSchema,
	}

	if config.DisplayName == "" {
		return nil, errors.New("display name is required")
	}

	if config.UniqueID == "" {
		return nil, errors.New("unique id is required")
	}

	// validate output schema
	if d.outputSchema == nil {
		return nil, errors.New("output schema is required")
	}

	// parse output schema
	if _, err := jsonschema.ParseSchema(d.outputSchema.Raw); err != nil {
		return nil, err
	}

	// If input schema provided, validate it
	if d.inputSchema != nil {
		if _, err := jsonschema.ParseSchema(d.inputSchema.Raw); err != nil {
			return nil, err
		}
	}

	for _, opt := range opts {
		opt(d)
	}

	// Validate that get function is set
	if d.getFunc == nil {
		return nil, errors.New("get function is required")
	}

	return d, nil
}

// Option is a function that configures a Definition
type Option func(*Definition)

// WithInstructions sets markdown documentation for the datasource
func WithInstructions(markdown string) Option {
	return func(d *Definition) {
		d.instructionsMarkdown = markdown
	}
}

// WithGetFunc sets the function that will be called to get data
func WithGetFunc(fn GetFunc) Option {
	return func(d *Definition) {
		d.getFunc = fn
	}
}

// WithCredentials sets the credentials required by this datasource
func WithCredentials(creds ...string) Option {
	return func(d *Definition) {
		d.credentialProviders = creds
	}
}

// UniqueID returns the unique identifier for this datasource
func (d *Definition) UniqueID() string {
	return d.uniqueID
}

// DisplayName returns the display name for this datasource
func (d *Definition) DisplayName() string {
	return d.displayName
}

// OutputSchema returns the schema that defines the output of the datasource
func (d *Definition) OutputSchema() *jsonschema.Schema {
	return d.outputSchema
}

// InputSchema returns the schema that defines the input parameters for the datasource
func (d *Definition) InputSchema() *jsonschema.Schema {
	return d.inputSchema
}

// InstructionsMarkdown returns the documentation for the datasource
func (d *Definition) InstructionsMarkdown() string {
	return d.instructionsMarkdown
}

// GetFunc returns the function that will be called to get data
func (d *Definition) GetFunc() GetFunc {
	return d.getFunc
}

// Credentials returns the credentials required by this datasource
func (d *Definition) CredentialProviders() []string {
	return d.credentialProviders
}

// JSON returns the JSON representation of the datasource definition
func (d *Definition) JSON() ([]byte, error) {
	data := map[string]any{
		"displayName": d.displayName,
		"uniqueID":    d.uniqueID,
	}

	// Add output schema
	if d.outputSchema != nil {
		outputSchema, err := d.outputSchema.Raw.MarshalJSON()
		if err != nil {
			return nil, err
		}
		data["outputSchema"] = json.RawMessage(outputSchema)
	}

	// Add input schema if provided
	if d.inputSchema != nil {
		inputSchema, err := d.inputSchema.Raw.MarshalJSON()
		if err != nil {
			return nil, err
		}
		data["inputSchema"] = json.RawMessage(inputSchema)
	}

	// Add instructions if any
	if d.instructionsMarkdown != "" {
		data["instructionsMarkdown"] = d.instructionsMarkdown
	}

	// Add credentials if any
	if len(d.credentialProviders) > 0 {
		data["credentialProviders"] = d.credentialProviders
	}

	return json.Marshal(data)
}
