package datasources

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/github"
	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/jsonschema"
)

// Schema for the GitHub Repository datasource
const (
	repositoryInputSchema = `
{
	"type": "object",
	"properties": {
		"organization": {
			"type": "string",
			"description": "GitHub organization name"
		}
	},
	"required": ["organization"],
	"additionalProperties": false
}`

	repositoryOutputSchema = `
{
	"type": "object",
	"properties": {
		"repositories": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"description": "Repository name"
					},
					"full_name": {
						"type": "string",
						"description": "Full repository name (org/repo)"
					},
					"description": {
						"type": "string",
						"description": "Repository description"
					},
					"default_branch": {
						"type": "string",
						"description": "Default branch of the repository"
					},
					"private": {
						"type": "boolean",
						"description": "Whether the repository is private"
					},
					"html_url": {
						"type": "string",
						"description": "Repository HTML URL"
					},
					"clone_url": {
						"type": "string",
						"description": "Repository clone URL (HTTPS)"
					},
					"ssh_url": {
						"type": "string",
						"description": "Repository SSH URL"
					}
				}
			},
			"description": "List of repositories in the organization"
		}
	},
	"additionalProperties": false
}`

	repositoryInstructionsMarkdown = `
# GitHub Repositories Datasource

Retrieves a list of all repositories in a GitHub organization.
`
)

// NewRepositoryDefinition creates a new GitHub Repository datasource definition
func NewRepositoryDefinition() (*datasource.Definition, error) {
	// Parse schemas
	inputSchema := jsonschema.MustParseSchema([]byte(repositoryInputSchema))
	outputSchema := jsonschema.MustParseSchema([]byte(repositoryOutputSchema))

	// Create the datasource definition
	return datasource.New(
		datasource.Config{
			DisplayName:  "GitHub Repositories",
			UniqueID:     "github-repositories",
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		datasource.WithInstructions(repositoryInstructionsMarkdown),
		datasource.WithGetFunc(getRepositories),
		datasource.WithCredentials(githubTokenKey),
	)
}

func getRepositories(ctx context.Context, req *datasource.WebhookRequest) (*datasource.WebhookResponse, error) {
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
	
	organization := req.Args["organization"].(string)

	// Create GitHub client
	client := getGitHubClient(githubToken)

	// List repositories
	var repositories []map[string]any

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, organization, opt)
		if err != nil {
			return &datasource.WebhookResponse{
				Error:   fmt.Errorf("failed to list repositories: %w", err),
				Message: "Error retrieving GitHub repositories",
			}, nil
		}

		for _, repo := range repos {
			repositories = append(repositories, map[string]any{
				"name":           repo.GetName(),
				"full_name":      repo.GetFullName(),
				"description":    repo.GetDescription(),
				"default_branch": repo.GetDefaultBranch(),
				"private":        repo.GetPrivate(),
				"html_url":       repo.GetHTMLURL(),
				"clone_url":      repo.GetCloneURL(),
				"ssh_url":        repo.GetSSHURL(),
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	return &datasource.WebhookResponse{
		Data: map[string]any{
			"repositories": repositories,
		},
	}, nil
}
