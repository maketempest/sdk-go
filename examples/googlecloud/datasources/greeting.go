package datasources

import (
	"context"
	"fmt"

	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/jsonschema"
)

// Schema for the greeting datasource
const (
	greetingInputSchema = `
{
	"$schema": "http://json-schema.org/draft-07/schema",
	"type": "object",
	"properties": {
		"name": {
			"title": "Name",
			"description": "Your name",
			"type": "string"
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`

	greetingOutputSchema = `
{
	"$schema": "http://json-schema.org/draft-07/schema",
	"type": "object",
	"properties": {
		"greeting": {
			"title": "Greeting",
			"description": "Greeting message",
			"type": "string"
		}
	},
	"additionalProperties": false
}`

	greetingInstructionsMarkdown = `
# Greeting Datasource

A simple datasource that returns a greeting message with your name.
`
)

// NewGreetingDefinition creates a new greeting datasource definition
func NewGreetingDefinition() (*datasource.Definition, error) {
	// Parse schemas
	inputSchema := jsonschema.MustParseSchema([]byte(greetingInputSchema))
	outputSchema := jsonschema.MustParseSchema([]byte(greetingOutputSchema))

	// Create the datasource definition
	return datasource.New(
		datasource.Config{
			DisplayName:  "Greeting",
			UniqueID:     "greeting",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		datasource.WithInstructions(greetingInstructionsMarkdown),
		datasource.WithGetFunc(getGreeting),
	)
}

func getGreeting(ctx context.Context, req *datasource.WebhookRequest) (*datasource.WebhookResponse, error) {
	// Get name from request parameters
	name, ok := req.Params["name"].(string)
	if !ok {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("name parameter is required"),
			Message: "Name parameter is required and must be a string",
		}, nil
	}

	// Generate greeting message
	greeting := fmt.Sprintf("Hello, %s! Welcome to the Google Cloud example.", name)

	// Return the response
	return &datasource.WebhookResponse{
		Data: map[string]any{
			"greeting": greeting,
		},
	}, nil
}
