package resources

import (
	"context"
	"fmt"

	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
)

// Hardcoded data for demonstration
var exampleRepo = map[string]any{
	"name": "example-repo",
	"url":  "https://github.com/example/example-repo",
}

func repositoryToResource(id string, props map[string]any) *resource.Resource {
	name := id
	if n, ok := props["name"].(string); ok {
		name = n
	}
	return &resource.Resource{
		ResourceRef: resource.ResourceRef{
			ExternalID: id,
			Name:       name,
		},
		DisplayName: name,
		Category:    resource.CategoryCodeRepository,
		Properties:  props,
		Links: []*resource.Link{
			{
				Title: "GitHub Repository",
				URL:   props["url"].(string),
				Type:  resource.LinkTypeWebsite,
			},
		},
	}
}

func createRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	name := req.Args["name"].(string)
	newRepoID := fmt.Sprintf("%s-123", name)
	newRepoProps := map[string]any{
		"name": name,
		"url":  fmt.Sprintf("https://github.com/example/%s", name),
	}
	return resource.NewSingleResourceResponse(repositoryToResource(newRepoID, newRepoProps), fmt.Sprintf("Created repository %s", name), nil), nil
}

func listRepositories(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	items := []*resource.Resource{
		repositoryToResource("example-repo-123", exampleRepo),
	}
	return resource.NewResourceListResponse(items, len(items), 10, "", fmt.Sprintf("Found %d repositories", len(items)), nil), nil
}

func readRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	if req.Resource == nil || req.Resource.ExternalID == "" {
		return nil, fmt.Errorf("resource ID is missing")
	}
	return resource.NewSingleResourceResponse(repositoryToResource(req.Resource.ExternalID, exampleRepo), fmt.Sprintf("Found repository %s", req.Resource.ExternalID), nil), nil
}

func deleteRepository(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	if req.Resource == nil || req.Resource.ExternalID == "" {
		return nil, fmt.Errorf("resource ID is missing")
	}
	return &resource.OperationResponse{
		Data: &resource.Resource{
			ResourceRef: resource.ResourceRef{
				ExternalID: req.Resource.ExternalID,
				Name:       req.Resource.Name,
			},
		},
	}, nil
}

func NewRepositoryResource() (*resource.Definition, error) {
	repoDef, err := resource.NewDefinition(
		resource.DefinitionConfig{
			DisplayName:    "GitHub Repository",
			UniqueID:       "github_repository", // Simple ID for example
			Properties:     jsonschema.MustParseSchema([]byte(repositoryPropertiesSchema)),
			LifecycleStage: resource.LifecycleStageDeploy,
		},
		resource.WithInstructions(repositoryInstructionsMarkdown),
		resource.WithCategories(resource.CategoryCodeRepository),
		// Add WithCredentials if needed, e.g., resource.WithCredentials("github_pat")
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
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.Read,
			Priority: 100,
		}),
	)

	repoDef.RegisterOperation(
		"delete",
		deleteRepository,
		resource.Op.On(resource.CanonicalOperation{
			Type:     resource.Delete,
			Priority: 100,
		}),
	)

	return repoDef, nil
}
