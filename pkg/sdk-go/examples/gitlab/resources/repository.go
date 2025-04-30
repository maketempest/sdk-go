package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Key for accessing GitLab token from credentials
const gitlabTokenKey = "gitlab_token"

// createGitLabClient creates a GitLab client using the provided token
func createGitLabClient(token string) (*gitlab.Client, error) {
	client, err := gitlab.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}
	return client, nil
}

// projectToResource converts a GitLab project to a resource
func projectToResource(project *gitlab.Project) *resource.Resource {
	return &resource.Resource{
		ResourceRef: resource.ResourceRef{
			ExternalID: project.PathWithNamespace, // unique identifier for the repository
			Name:       project.Name,
		},
		DisplayName: project.PathWithNamespace,
		Category:    resource.CategoryCodeRepository,
		Properties: map[string]any{
			"name":           project.Name,
			"url":            project.WebURL,
			"http_clone_url": project.HTTPURLToRepo,
			"ssh_clone_url":  project.SSHURLToRepo,
			"description":    project.Description,
			"externalId":     project.PathWithNamespace,
			"default_branch": project.DefaultBranch,
			"provider":       "gitlab",
		},
		Links: []*resource.Link{
			{
				Title: "GitLab Repository",
				URL:   project.WebURL,
				Type:  resource.LinkTypeWebsite,
			},
		},
	}
}

func createRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[gitlabTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}

	client, err := createGitLabClient(token)
	if err != nil {
		return nil, err
	}

	name := req.Args["name"].(string)
	visibility := gitlab.PrivateVisibility
	if vis, ok := req.Args["visibility"].(string); ok && vis != "" {
		switch vis {
		case "public":
			visibility = gitlab.PublicVisibility
		case "internal":
			visibility = gitlab.InternalVisibility
		case "private":
			visibility = gitlab.PrivateVisibility
		}
	}

	namespace := ""
	if ns, ok := req.Args["namespace"].(string); ok && ns != "" {
		namespace = ns
	}

	var namespaceID int
	if namespace != "" {
		// Look up the namespace ID
		namespaces, _, err := client.Namespaces.ListNamespaces(&gitlab.ListNamespacesOptions{Search: &namespace})
		if err != nil {
			return nil, fmt.Errorf("failed to look up namespace: %w", err)
		}

		if len(namespaces) == 0 {
			return nil, fmt.Errorf("namespace %s not found", namespace)
		}

		// Use the first matching namespace
		namespaceID = namespaces[0].ID
	}

	// Initialize with README?
	initWithReadme := true
	if init, ok := req.Args["initialize_with_readme"].(bool); ok {
		initWithReadme = init
	}

	// Create the project
	opts := &gitlab.CreateProjectOptions{
		Name:                 gitlab.String(name),
		NamespaceID:          gitlab.Int(namespaceID),
		Visibility:           gitlab.Visibility(visibility),
		InitializeWithReadme: gitlab.Bool(initWithReadme),
	}

	project, _, err := client.Projects.CreateProject(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return resource.NewSingleResourceResponse(
		projectToResource(project),
		fmt.Sprintf("Created GitLab project %s", project.PathWithNamespace),
		nil,
	), nil
}

func listRepositories(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[gitlabTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}

	client, err := createGitLabClient(token)
	if err != nil {
		return nil, err
	}

	// Determine namespace filter
	namespace := ""
	if ns, ok := req.Args["namespace"].(string); ok && ns != "" {
		namespace = ns
	}

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

	// List projects
	opts := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    page,
			PerPage: perPage,
		},
	}

	// Apply namespace filter if provided
	if namespace != "" {
		opts.Search = gitlab.String(namespace)
	}

	projects, _, err := client.Projects.ListProjects(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	// Convert projects to resources
	items := make([]*resource.Resource, len(projects))
	for i, project := range projects {
		items[i] = projectToResource(project)
	}

	nextCursor := ""
	if len(projects) == perPage {
		nextCursor = fmt.Sprintf("%d", page+1)
	}

	return resource.NewResourceListResponse(
		items,
		len(items),
		perPage,
		nextCursor,
		fmt.Sprintf("Found %d projects", len(items)),
		nil,
	), nil
}

func readRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[gitlabTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}

	client, err := createGitLabClient(token)
	if err != nil {
		return nil, err
	}

	externalId, ok := req.Args["externalId"].(string)
	if !ok || externalId == "" {
		return nil, fmt.Errorf("externalId is required")
	}

	var project *gitlab.Project
	// Check if the externalId is a numeric project ID
	if id, err := strconv.Atoi(externalId); err == nil {
		project, _, err = client.Projects.GetProject(id, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get project with ID %d: %w", id, err)
		}
	} else {
		// Assume it's a path with namespace (e.g., "group/project")
		project, _, err = client.Projects.GetProject(externalId, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get project with path %s: %w", externalId, err)
		}
	}

	return resource.NewSingleResourceResponse(
		projectToResource(project),
		fmt.Sprintf("Found GitLab project %s", project.PathWithNamespace),
		nil,
	), nil
}

func deleteRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	token, ok := req.Credentials.Values[gitlabTokenKey]
	if !ok || token == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}

	client, err := createGitLabClient(token)
	if err != nil {
		return nil, err
	}

	externalId, ok := req.Args["externalId"].(string)
	if !ok || externalId == "" {
		return nil, fmt.Errorf("externalId is required")
	}

	var projectID interface{}
	// Check if the externalId is a numeric project ID
	if id, err := strconv.Atoi(externalId); err == nil {
		projectID = id
	} else {
		// Assume it's a path with namespace (e.g., "group/project")
		projectID = externalId
	}

	// Updated to include required empty options parameter
	_, err = client.Projects.DeleteProject(projectID, &gitlab.DeleteProjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to delete project %v: %w", projectID, err)
	}

	return &resource.OperationResponse{
		Message: fmt.Sprintf("Deleted GitLab project %s", externalId),
	}, nil
}

func NewRepositoryResource() (*resource.Definition, error) {
	repoDef, err := resource.NewDefinition(
		resource.DefinitionConfig{
			DisplayName:    "GitLab Repository",
			UniqueID:       "gitlab_repository",
			Properties:     jsonschema.MustParseSchema([]byte(repositoryPropertiesSchema)),
			LifecycleStage: resource.LifecycleStageDeploy,
		},
		resource.WithInstructions(repositoryInstructionsMarkdown),
		resource.WithCategories(resource.CategoryCodeRepository),
		resource.WithCredentials("gitlab_api_token"),
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
