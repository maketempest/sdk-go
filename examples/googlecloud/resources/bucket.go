package googlecloud

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
	"google.golang.org/api/storage/v1"
)

// For simplicity in this example, we're using hardcoded values
// In a real implementation, these would be properly managed
var (
	serviceAccountCreds = "SERVICE_ACCOUNT_CREDS"
	projectID           = "tempest-sandbox"
)

func bucketToResource(bucket *storage.Bucket) *resource.Resource {
	return &resource.Resource{
		ExternalID:  bucket.Id,
		DisplayName: bucket.Name,
		Name:        bucket.Name,
		Category:    resource.CategoryStorage,
		Links: []*resource.Link{
			{
				Title: "Google Cloud Console",
				URL:   "https://console.cloud.google.com/storage/browser/" + bucket.Name,
				Type:  resource.LinkTypeWebsite,
			},
		},
		Properties: map[string]any{
			"id":            bucket.Id,
			"location":      bucket.Location,
			"storage_class": bucket.StorageClass,
		},
	}
}

func getAuthOption() (option.ClientOption, error) {
	b64, err := base64.StdEncoding.DecodeString(serviceAccountCreds)
	if err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}

	return option.WithCredentialsJSON(b64), nil
}

func bucketHealthCheck(ctx context.Context) (*resource.HealthCheckResponse, error) {
	// TODO: Check if the bucket exists and is healthy
	return &resource.HealthCheckResponse{
		Status: resource.HealthCheckStatusHealthy,
	}, nil
}

func createBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption()
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	service, err := storage.NewService(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}

	name := req.Args["name"].(string)
	location := "US"
	if loc, ok := req.Args["location"].(string); ok && loc != "" {
		location = loc
	}

	storageClass := "STANDARD"
	if sc, ok := req.Args["storage_class"].(string); ok && sc != "" {
		storageClass = sc
	}

	bucket := &storage.Bucket{
		Name:         name,
		Location:     location,
		StorageClass: storageClass,
	}

	createdBucket, err := service.Buckets.Insert(projectID, bucket).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return &resource.OperationResponse{
		Resource: bucketToResource(createdBucket),
	}, nil
}

// func listBuckets(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
// 	opt, err := getAuthOption()
// 	if err != nil {
// 		return nil, fmt.Errorf("create auth option: %w", err)
// 	}

// 	service, err := storage.NewService(ctx, opt)
// 	if err != nil {
// 		return nil, fmt.Errorf("create service: %w", err)
// 	}

// 	serviceReq := service.Buckets.List(projectID)

// 	// OPERATION REQUEST DOES NOT HANDLE PAGINATION
// 	if req.NextToken != "" {
// 		serviceReq.PageToken(req.NextToken)
// 	}

// 	buckets, err := serviceReq.Context(ctx).Do()
// 	if err != nil {
// 		return nil, fmt.Errorf("list buckets: %w", err)
// 	}

// 	var items []*resource.Resource
// 	for _, i := range buckets.Items {
// 		items = append(items, bucketToResource(i))
// 	}

// 	// OPERATION RESPONSE CONTAINS A SINGLE RESOURCE
// 	out := &resource.OperationResponse{
// 		Resource: items,
// 	}

// 	if buckets.NextPageToken != "" {
// 		out.NextToken = buckets.NextPageToken
// 	}

// 	return out, nil
// }

func readBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption()
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	service, err := storage.NewService(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}

	bucket, err := service.Buckets.Get(req.Resource.ExternalID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get bucket: %w", err)
	}

	return &resource.OperationResponse{
		Resource: bucketToResource(bucket),
	}, nil
}

func deleteBucket(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	opt, err := getAuthOption()
	if err != nil {
		return nil, fmt.Errorf("create auth option: %w", err)
	}

	service, err := storage.NewService(ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}

	objects, err := service.Objects.List(req.Resource.ExternalID).Context(ctx).Versions(true).Do()
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}

	g, errGroupCtx := errgroup.WithContext(ctx)
	for _, object := range objects.Items {
		object := object
		g.Go(func() error {
			return service.Objects.Delete(req.Resource.ExternalID, object.Name).Context(errGroupCtx).Generation(object.Generation).Do()
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("delete objects: %w", err)
	}

	err = service.Buckets.Delete(req.Resource.ExternalID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("delete bucket: %w", err)
	}

	return &resource.OperationResponse{
		Resource: req.Resource,
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
		// TODO: check if more options are needed
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

	// TODO: How to handle list operation when operation response assumes a resource is single object?
	// bucketDef.RegisterOperation(
	// 	"list",
	// 	listBuckets,
	// 	resource.Op.On(resource.CanonicalOperation{
	// 		Type:        resource.List,
	// 		Priority:    100,
	// 		Concurrency: 1,
	// 	}),
	// )

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
