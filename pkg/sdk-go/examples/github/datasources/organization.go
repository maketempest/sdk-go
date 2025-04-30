package datasources

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/github"
	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/jsonschema"
	"golang.org/x/oauth2"
)

// Schema for the GitHub Organization datasource
const (
	organizationInputSchema = `
{
	"type": "object",
	"properties": {},
	"additionalProperties": false
}`

	organizationOutputSchema = `
{
	"type": "object",
	"properties": {
		"organizations": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"login": {
						"type": "string",
						"description": "Organization login/name"
					},
					"description": {
						"type": "string",
						"description": "Organization description"
					}
				}
			},
			"description": "List of GitHub organizations the user is a member of"
		}
	},
	"additionalProperties": false
}`

	organizationInstructionsMarkdown = `
# GitHub Organizations Datasource

Retrieves a list of all GitHub organizations the authenticated user is a member of.
`
)

const (
	githubTokenKey = "github-token"
)

// NewOrganizationDefinition creates a new GitHub Organization datasource definition
func NewOrganizationDefinition() (*datasource.Definition, error) {
	// Parse schemas
	inputSchema := jsonschema.MustParseSchema([]byte(organizationInputSchema))
	outputSchema := jsonschema.MustParseSchema([]byte(organizationOutputSchema))

	// Create the datasource definition
	return datasource.New(
		datasource.Config{
			DisplayName:  "GitHub Organizations",
			UniqueID:     "github-organizations",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		datasource.WithInstructions(organizationInstructionsMarkdown),
		datasource.WithGetFunc(getOrganizations),
		datasource.WithCredentials(githubTokenKey),
	)
}

func getGitHubClient(token string) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	return github.NewClient(tc)
}

func getOrganizations(ctx context.Context, req *datasource.WebhookRequest) (*datasource.WebhookResponse, error) {
	// Get auth credentials from the request
	if req.Credentials == nil || req.Credentials.Values == nil {
		return &datasource.WebhookResponse{
			Error:   errors.New("missing credentials"),
			Message: "GitHub token is required",
		}, nil
	}

	githubToken, ok := req.Credentials.Values[githubTokenKey]
	if !ok {
		return &datasource.WebhookResponse{
			Error:   errors.New("missing GitHub token"),
			Message: "GitHub token is required for authentication",
		}, nil
	}

	// Create GitHub client
	client := getGitHubClient(githubToken)

	// List organizations
	orgs, _, err := client.Organizations.List(ctx, "", nil)
	if err != nil {
		return &datasource.WebhookResponse{
			Error:   fmt.Errorf("failed to list organizations: %w", err),
			Message: "Error retrieving GitHub organizations",
		}, nil
	}

	// Process the response
	var organizations []map[string]any
	for _, org := range orgs {
		organizations = append(organizations, map[string]any{
			"login":       org.GetLogin(),
			"description": org.GetDescription(),
		})
	}

	return &datasource.WebhookResponse{
		Data: map[string]any{
			"organizations": organizations,
		},
	}, nil
}
