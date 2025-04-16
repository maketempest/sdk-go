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
	resources "github.com/tempestdx/sdk-go/examples/googlecloud/resources"
)

func main() {
	// Create the bucket resource definition
	bucketDef, err := resources.NewBucketDefinition()
	if err != nil {
		panic(err)
	}

	// Create the Google Cloud app
	googleCloudApp, err := app.New(
		app.Config{
			Name: "google-cloud",
		},
		app.WithResource(bucketDef),
	)
	if err != nil {
		panic(err)
	}

	serviceAccountCredentialProvider := credentials.NewServiceAccountCredentialProvider()

	// Create and configure the agent
	agentInstance, err := agent.New(
		agent.WithServerAddr(":8080"),
		agent.WithAPIKey("eyJhbGciOiJFUzUxMiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ4ZW4xNTduWG8wdjlLIiwiZXhwIjoxNzc1Nzg4MzI0LCJpc3MiOiJUZW1wZXN0IExhYnMsIEluYy4gKGxvY2FsKSIsImp0aSI6InBlbldkYTd5WDBROG4iLCJzdWIiOiJ2cDU1eXliQmVsbmRhIn0.AVf-nIhR0xzvv5NHSEq0Mg2NmrJuat5yX7ieE3FKNrx3hVIlSOlJDCtqVGGuP0sDuHHJ8KU3As9j3VllUX2KjDaYAK9pIuecQRE1YVQPg9TbOX20o1m_rtZ5qhdtGTFr5As2SjWFSrQNuTHpO7hPjyQ2DUWFKvvyXSieB84IBX_6Jv0e"),
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
	agentInstance.RegisterApp(googleCloudApp)
	agentInstance.RegisterCredentials(serviceAccountCredentialProvider)

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
