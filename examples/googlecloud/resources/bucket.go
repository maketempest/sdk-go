package googlecloud

import (
	"context"
	"encoding/base64"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// For simplicity in this example, we're using hardcoded values
// In a real implementation, these would be properly managed
var (
	projectID   = "tempest-sandbox"
	saApikeyKey = "google-cloud-storage-api-key"
)

func bucketToResource(bucket *storage.BucketAttrs) *resource.Resource {
	return &resource.Resource{
		ResourceRef: resource.ResourceRef{
			ExternalID: bucket.Name,
			Name:       bucket.Name,
		},
		DisplayName: bucket.Name,
		Category:    resource.CategoryStorage,
		Links: []*resource.Link{
			{
				Title: "Google Cloud Console",
				URL:   "https://console.cloud.google.com/storage/browser/" + bucket.Name,
				Type:  resource.LinkTypeWebsite,
			},
		},
		Properties: map[string]any{
			"created":           bucket.Created,
			"updated":           bucket.Updated,
			"location":          bucket.Location,
			"storageClass":      bucket.StorageClass,
			"versioningEnabled": bucket.VersioningEnabled,
		},
	}
}

func getAuthOption(serviceAccountCreds string) (option.ClientOption, error) {
	b64, err := base64.StdEncoding.DecodeString(serviceAccountCreds)
	if err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}

	return option.WithCredentialsJSON(b64), nil
}

func bucketHealthCheck(ctx context.Context) (*resource.HealthCheckResponse, error) {
	return &resource.HealthCheckResponse{
		Status: resource.HealthCheckStatusHealthy,
	}, nil
}

func createBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption(req.Credentials.Values[saApikeyKey])

	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	client, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	name := req.Args["name"].(string)
	location := "US"
	if loc, ok := req.Args["location"].(string); ok && loc != "" {
		location = loc
	}

	storageClass := "STANDARD"
	if sc, ok := req.Args["storageClass"].(string); ok && sc != "" {
		storageClass = sc
	}

	bucketAttr := &storage.BucketAttrs{
		Name:         name,
		Location:     location,
		StorageClass: storageClass,
	}

	bucket := client.Bucket(name)
	err = bucket.Create(ctx, projectID, bucketAttr)

	if err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	bucketAttr, err = bucket.Attrs(ctx)

	if err != nil {
		return nil, fmt.Errorf("getting bucket attributes: %w", err)
	}

	return &resource.OperationResponse{
		Data: bucketToResource(bucketAttr),
	}, nil
}

func listBuckets(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption(req.Credentials.Values[saApikeyKey])
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}
	client, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}
	defer client.Close()

	bucketsIter := client.Buckets(ctx, projectID)
	pageSize := 5
	if req.Pagination != nil {
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
		if req.Pagination.Cursor != "" {
			bucketsIter.PageInfo().Token = req.Pagination.Cursor
		}
	}
	// Collect bucket attributes
	var items []*resource.Resource
	var nextCursor string
	for {
		bucketAttrs, err := bucketsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list buckets: %w", err)
		}
		items = append(items, bucketToResource(bucketAttrs))
		if len(items) >= pageSize {
			// Get the token for the next page
			nextCursor = bucketsIter.PageInfo().Token
			break
		}
	}
	totalCount := bucketsIter.PageInfo().MaxSize
	if nextCursor != "" {
		totalCount = -1
	}
	return resource.NewResourceListResponse(items, totalCount, pageSize, nextCursor, fmt.Sprintf("Found %d buckets", len(items)), nil), nil
}

func readBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption(req.Credentials.Values[saApikeyKey])
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	// Create a new client with the Cloud Storage package
	client, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create cloud storage client: %w", err)
	}
	defer client.Close()

	if req.Resource == nil || req.Resource.ExternalID == "" {
		return nil, fmt.Errorf("resource ID is missing")
	}

	bucketName := req.Resource.ExternalID
	bkt := client.Bucket(bucketName)
	attrs, err := bkt.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get bucket attributes: %w", err)
	}

	return resource.NewSingleResourceResponse(bucketToResource(attrs), fmt.Sprintf("Found bucket %s", req.Resource.ExternalID), nil), nil
}

func deleteBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	if req.Resource == nil {
		return nil, fmt.Errorf("resource is nil")
	}

	opt, err := getAuthOption(req.Credentials.Values[saApikeyKey])
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	client, err := storage.NewClient(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	err = client.Bucket(req.Resource.ExternalID).Delete(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete bucket: %w", err)
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

// NewBucketDefinition creates and returns a new bucket resource definition
func NewBucketDefinition() (*resource.Definition, error) {
	bucketDef, err := resource.NewDefinition(
		resource.DefinitionConfig{
			DisplayName:    "Storage Bucket",
			UniqueID:       "google-cloud.storage-bucket",
			Properties:     jsonschema.MustParseSchema([]byte(bucketPropertiesSchema)),
			LifecycleStage: resource.LifecycleStageDeploy,
		},
		resource.WithHealthCheck(bucketHealthCheck),
		resource.WithDefaultLinks(
			resource.Link{
				Title:    "Google Cloud Storage Documentation",
				URL:      "https://cloud.google.com/storage/docs",
				Type:     resource.LinkTypeWebsite,
				Category: resource.LinkCategoryDocumentation,
			},
		),
		resource.WithInstructions(bucketInstructionsMarkdown),
		resource.WithCategories(resource.CategoryStorage),
		resource.WithCredentials(saApikeyKey),
	)
	if err != nil {
		return nil, err
	}

	// Register operations
	bucketDef.RegisterOperation(
		"create",
		createBucket,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(createBucketSchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Create,
			Priority:    100,
			Concurrency: 1,
		}),
		resource.Op.EnableAction(resource.ActionConfig{
			Title:                "Create Bucket",
			Description:          "Create a new Google Cloud Storage bucket.",
			RequiresConfirmation: true,
		}),
	)

	bucketDef.RegisterOperation(
		"list",
		listBuckets,
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.List,
			Priority:    100,
			Concurrency: 1,
		}),
	)

	bucketDef.RegisterOperation(
		"read",
		readBucket,
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Read,
			Priority:    100,
			Concurrency: 1,
		}),
	)

	bucketDef.RegisterOperation(
		"delete",
		deleteBucket,
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Delete,
			Priority:    100,
			Concurrency: 1,
		}),
		resource.Op.EnableAction(resource.ActionConfig{
			Title:                "Delete Bucket",
			Description:          "Delete a Google Cloud Storage bucket and all its contents.",
			RequiresConfirmation: true,
		}),
	)

	return bucketDef, nil
}
