package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/tempestdx/sdk-go/agent"
	"github.com/tempestdx/sdk-go/app"
	credentials "github.com/tempestdx/sdk-go/examples/googlecloud/credentials"
	datasources "github.com/tempestdx/sdk-go/examples/googlecloud/datasources"
	resources "github.com/tempestdx/sdk-go/examples/googlecloud/resources"
)

func main() {
	// Create the bucket resource definition
	bucketDef, err := resources.NewBucketDefinition()
	if err != nil {
		panic(err)
	}

	// Create the GCP Projects datasource definition
	projectsDef, err := datasources.NewProjectsDefinition()
	if err != nil {
		panic(err)
	}

	// Create the Google Cloud app
	googleCloudApp, err := app.New(
		app.Config{
			ID:   "google-cloud",
			Name: "Google Cloud",
		},
		app.WithIcon("examples/googlecloud/logo.svg"),
		app.WithResource(bucketDef),
		app.WithDataSource(projectsDef),
	)
	if err != nil {
		panic(err)
	}

	serviceAccountCredentialProvider := credentials.NewServiceAccountCredentialProvider()

	// Create and configure the agent
	agentInstance, err := agent.New(
		agent.WithServerAddr(":8080"),
		agent.WithAPIKey("tempest-api-key"),
		agent.WithAPIURL("http://localhost:8040"),
		agent.WithWorkers(1),
		agent.WithResultWorkers(1),
		agent.WithPollTimeout(5*time.Second),
		agent.WithLogger(slog.New(
			slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}),
		)),
	)
	if err != nil {
		panic(err)
	}

	// Register the Google Cloud app with the agent
	agentInstance.RegisterCredentials(serviceAccountCredentialProvider)
	agentInstance.RegisterApp(googleCloudApp)

	fmt.Println("Starting Google Cloud agent on port 8080...")
	if err := agentInstance.Run(); err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Println("Google Cloud agent stopped")

	// Graceful shutdown
	ctx := context.Background()
	err = agentInstance.Stop(ctx)
	if err != nil {
		log.Fatalf("Failed to stop agent: %v", err)
	}
}
