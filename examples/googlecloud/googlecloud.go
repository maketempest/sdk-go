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
	"github.com/tempestdx/sdk-go/examples/googlecloud/resources"
)

func main() {
	// Create the bucket resource definition
	bucketDef, err := googlecloud.NewBucketDefinition()
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

	// Create and configure the agent
	agentInstance, err := agent.New(
		agent.WithServerAddr(":8080"),
		agent.WithWorkers(5),
		agent.WithResultWorkers(2),
		agent.WithPollTimeout(100*time.Millisecond),
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
	agentInstance.RegisterApp(googleCloudApp, []string{"v1"})

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
