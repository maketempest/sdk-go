package datasources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/jsonschema"
	resourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
)

// Schema for the GCP Projects datasource
const (
	projectsInputSchema = `
{
	"type": "object",
	"properties": {},
	"additionalProperties": false
}`

	projectsOutputSchema = `
{
	"type": "object",
	"properties": {
		"projects": {
			"type": "string",
			"description": "JSON string representing Google Cloud projects indexed by project ID"
		}
	},
	"additionalProperties": false
}`

	projectsInstructionsMarkdown = `
# Google Cloud Projects Datasource

Retrieves a list of Google Cloud projects accessible to the authenticated service account.
`
)

const (
	saApikeyKey = "google-cloud-storage-api-key"
)

// NewProjectsDefinition creates a new Google Cloud Projects datasource definition
func NewProjectsDefinition() (*datasource.Definition, error) {
	// Parse schemas
	inputSchema := jsonschema.MustParseSchema([]byte(projectsInputSchema))
	outputSchema := jsonschema.MustParseSchema([]byte(projectsOutputSchema))

	// Create the datasource definition
	return datasource.New(
		datasource.Config{
			DisplayName:  "Google Cloud Projects",
			UniqueID:     "google-cloud-projects",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		datasource.WithInstructions(projectsInstructionsMarkdown),
		datasource.WithGetFunc(getProjects),
		datasource.WithCredentials(saApikeyKey),
	)
}

func getAuthOption(serviceAccountCreds string) (option.ClientOption, error) {
	b64, err := base64.StdEncoding.DecodeString(serviceAccountCreds)
	if err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}

	return option.WithCredentialsJSON(b64), nil
}

func getProjects(ctx context.Context, req *datasource.WebhookRequest) (*datasource.WebhookResponse, error) {
	// Get auth credentials from the request
	if req.Credentials == nil || req.Credentials.Values == nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("missing credentials"),
			Message: "Service account credentials are required",
		}, nil
	}

	serviceAccountCreds, ok := req.Credentials.Values[saApikeyKey]
	if !ok {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("missing service account credentials"),
			Message: "Google Cloud service account credentials are required",
		}, nil
	}

	// Get auth option
	opt, err := getAuthOption(serviceAccountCreds)
	if err != nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("failed to create auth option: %w", err),
			Message: "Failed to process service account credentials",
		}, nil
	}

	// Create GCP Resource Manager service
	service, err := resourcemanager.NewService(ctx, opt)
	if err != nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("failed to create resource manager service: %w", err),
			Message: "Failed to initialize Google Cloud Resource Manager service",
		}, nil
	}
	// List the projects
	listCall := service.Projects.List()

	resp, err := listCall.Context(ctx).Do()
	if err != nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("failed to list projects: %w", err),
			Message: "Error retrieving Google Cloud projects",
		}, nil
	}

	// Process the response
	projects := make(map[string]map[string]any)
	for _, project := range resp.Projects {
		projects[project.ProjectId] = map[string]any{
			"id":   project.ProjectId,
			"name": project.Name,
		}
	}

	// Marshal projects map to JSON string
	projectsJSON, err := json.Marshal(projects)
	if err != nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("failed to marshal projects to JSON: %w", err),
			Message: "Error processing project data",
		}, nil
	}

	return &datasource.WebhookResponse{
		Data: map[string]any{
			"projects": string(projectsJSON),
		},
	}, nil
}
