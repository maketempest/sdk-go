package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/github"
	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
	"golang.org/x/oauth2"
)

// Key for accessing GitHub token from credentials
const githubTokenKey = "github_token"

// createGitHubClient creates a GitHub client using the provided token
func createGitHubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// repositoryToResource converts a GitHub repository to a resource
func repositoryToResource(repo *github.Repository) *resource.Resource {
	return &resource.Resource{
		ResourceRef: resource.ResourceRef{
			ExternalID: repo.GetFullName(), // unique identifier for the repository
			Name:       repo.GetName(),
		},
		DisplayName: repo.GetFullName(),
		Category:    resource.CategoryCodeRepository,
		Properties: map[string]any{
			"name":           repo.GetName(),
			"url":            repo.GetHTMLURL(),
			"http_clone_url": repo.GetCloneURL(),
			"ssh_clone_url":  repo.GetSSHURL(),
			"description":    repo.GetDescription(),
			"externalId":     repo.GetFullName(),
			"default_branch": repo.GetDefaultBranch(),
			"provider":       "github",
		},
		Links: []*resource.Link{
			{
				Title: "GitHub Repository",
				URL:   repo.GetHTMLURL(),
				Type:  resource.LinkTypeWebsite,
			},
		},
	}
}

func createRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[githubTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("github token is required")
	}

	client := createGitHubClient(ctx, token)

	name := req.Args["name"].(string)
	visibility := "private"
	if vis, ok := req.Args["visibility"].(string); ok && vis != "" {
		visibility = vis
	}

	orgName := ""
	if org, ok := req.Args["organization"].(string); ok && org != "" {
		orgName = org
	} else {
		// Try to get the authenticated user
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get authenticated user: %w", err)
		}
		orgName = user.GetLogin()
	}

	// Create the repository
	repo := &github.Repository{
		Name:     github.String(name),
		AutoInit: github.Bool(true),
		Private:  github.Bool(visibility == "private"),
	}

	// Standard repository creation
	var newRepo *github.Repository
	var err error

	if orgName != "" {
		newRepo, _, err = client.Repositories.Create(ctx, orgName, repo)
	} else {
		newRepo, _, err = client.Repositories.Create(ctx, "", repo)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	// Inform user about missing Dependabot functionality
	if enableDependabot, ok := req.Args["enable_dependabot"].(bool); ok && enableDependabot {
		fmt.Printf("Note: Dependabot configuration not available in this go-github version\n")
	}

	return resource.NewSingleResourceResponse(
		repositoryToResource(newRepo),
		fmt.Sprintf("Created repository %s", newRepo.GetFullName()),
		nil,
	), nil
}

func listRepositories(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[githubTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("github token is required")
	}

	client := createGitHubClient(ctx, token)

	// Determine organization or user
	orgName := ""
	if org, ok := req.Args["organization"].(string); ok && org != "" {
		orgName = org
	}

	var repos []*github.Repository
	var err error

	page := 1
	perPage := 10
	if req.Pagination != nil {
		if req.Pagination.PageSize > 0 {
			perPage = req.Pagination.PageSize
		}
		if req.Pagination.Cursor != "" {
			if _, err := fmt.Sscanf(req.Pagination.Cursor, "%d", &page); err != nil {
				return nil, fmt.Errorf("invalid cursor format: %w", err)
			}
		}
	}

	opt := &github.ListOptions{
		Page:    page,
		PerPage: perPage,
	}

	if orgName != "" {
		// List repositories for organization
		repos, _, err = client.Repositories.ListByOrg(ctx, orgName, &github.RepositoryListByOrgOptions{
			ListOptions: *opt,
		})
	} else {
		// List repositories for authenticated user
		repos, _, err = client.Repositories.List(ctx, "", &github.RepositoryListOptions{
			ListOptions: *opt,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	// Convert repos to resources
	items := make([]*resource.Resource, len(repos))
	for i, repo := range repos {
		items[i] = repositoryToResource(repo)
	}

	nextCursor := ""
	if len(repos) == perPage {
		nextCursor = fmt.Sprintf("%d", page+1)
	}

	return resource.NewResourceListResponse(
		items,
		len(items),
		perPage,
		nextCursor,
		fmt.Sprintf("Found %d repositories", len(items)),
		nil,
	), nil
}

func readRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[githubTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("github token is required")
	}

	client := createGitHubClient(ctx, token)

	externalId, ok := req.Args["externalId"].(string)
	if !ok || externalId == "" {
		return nil, fmt.Errorf("externalId is required")
	}

	// Extract owner and repo name from the externalId (format: owner/repo)
	parts := strings.Split(externalId, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository name format, expected owner/repo: %s", externalId)
	}

	repo, _, err := client.Repositories.Get(ctx, parts[0], parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	return resource.NewSingleResourceResponse(
		repositoryToResource(repo),
		fmt.Sprintf("Found repository %s", externalId),
		nil,
	), nil
}

func deleteRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[githubTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("github token is required")
	}

	client := createGitHubClient(ctx, token)

	externalId, ok := req.Args["externalId"].(string)
	if !ok || externalId == "" {
		return nil, fmt.Errorf("externalId is required")
	}

	// Extract owner and repo name from the externalId
	parts := strings.Split(externalId, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository name format, expected owner/repo: %s", externalId)
	}

	// Archive the repository instead of deleting it
	_, _, err := client.Repositories.Edit(ctx, parts[0], parts[1], &github.Repository{
		Archived: github.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to archive repository: %w", err)
	}

	return &resource.OperationResponse{
		Data: &resource.Resource{
			ResourceRef: resource.ResourceRef{
				ExternalID: externalId,
				Name:       parts[1],
			},
		},
		Message: fmt.Sprintf("Archived repository %s", externalId),
	}, nil
}

func NewRepositoryResource() (*resource.Definition, error) {
	repoDef, err := resource.NewDefinition(
		resource.DefinitionConfig{
			DisplayName:    "GitHub Repository",
			UniqueID:       "github_repository",
			Properties:     jsonschema.MustParseSchema([]byte(repositoryPropertiesSchema)),
			LifecycleStage: resource.LifecycleStageDeploy,
		},
		resource.WithInstructions(repositoryInstructionsMarkdown),
		resource.WithCategories(resource.CategoryCodeRepository),
		resource.WithCredentials("github_api_token"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base definition: %w", err)
	}

	// Register operations
	repoDef.RegisterOperation(
		"create",
		createRepository,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(createRepositorySchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.Create,
			Priority: 100,
		}),
	)

	repoDef.RegisterOperation(
		"list",
		listRepositories,
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.List,
			Priority: 100,
		}),
	)

	repoDef.RegisterOperation(
		"read",
		readRepository,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(readRepositorySchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.Read,
			Priority: 100,
		}),
	)

	repoDef.RegisterOperation(
		"delete",
		deleteRepository,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(deleteRepositorySchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.Delete,
			Priority: 100,
		}),
	)

	return repoDef, nil
}
