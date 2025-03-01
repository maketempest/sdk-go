package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tempestdx/sdk-go/agent"
	"github.com/tempestdx/sdk-go/app"
	"github.com/tempestdx/sdk-go/jsonschema"
	"github.com/tempestdx/sdk-go/resource"
)

const pgdbInstructionsMarkdown = `
# PostgreSQL Database

This resource allows you to create a new PostgreSQL database.
`
const pgdbResourceSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"name": {
			"title": "Name",
			"description": "The name of the database.",
			"type": "string",
			"default": "postgres"
		},
		"owner": {
			"title": "Owner",
			"description": "The owner of the database.",
			"type": "string",
			"default": "postgres"
		}
	},
	"additionalProperties": false
}`

const pgdbCreateArgsSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"name": {
		"title": "Name",
		"description": "The name of the database.",
		"type": "string",
		"default": "postgres"
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`

func pgdbHealthCheck(ctx context.Context) (*resource.HealthCheckResponse, error) {
	return &resource.HealthCheckResponse{
		Status: resource.HealthCheckStatusHealthy,
	}, nil
}

func pgdbCreate(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	// In a real implementation, we would connect to PostgreSQL and create a database
	fmt.Println("Creating PostgreSQL database:", req.Resource.Properties["name"])

	// For demo purposes, just return the resource with updated properties
	if req.Resource.Properties == nil {
		req.Resource.Properties = make(map[string]interface{})
	}

	// Add creation timestamp
	req.Resource.Properties["created_at"] = time.Now().Format(time.RFC3339)

	return &resource.OperationResponse{
		Resource: req.Resource,
	}, nil
}

func main() {
	// Create the PostgreSQL database resource definition
	pgdb, err := resource.NewDefinition(
		resource.DefinitionConfig{
			DisplayName:    "Database",
			UniqueID:       "postgres.database",
			Properties:     jsonschema.MustParseSchema([]byte(pgdbResourceSchema)),
			LifecycleStage: resource.LifecycleStageDeploy,
		},
		resource.WithHealthCheck(pgdbHealthCheck),
		resource.WithDefaultLinks(
			resource.Link{
				Title:    "PostgreSQL Documentation: CREATE DATABASE",
				URL:      "https://www.postgresql.org/docs/current/sql-createdatabase.html",
				Type:     resource.LinkTypeWebsite,
				Category: resource.LinkCategoryDocumentation,
			},
		),
		resource.WithInstructions(pgdbInstructionsMarkdown),
		resource.WithCategories(resource.CategoryDatabase),
	)
	if err != nil {
		panic(err)
	}

	// Register the create operation
	pgdb.RegisterOperation(
		"create_db",
		pgdbCreate,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(pgdbCreateArgsSchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Create,
			Concurrency: 1,
		}),
		resource.Op.EnableAction(resource.ActionConfig{
			Title:                "Create Database",
			Description:          "Create a new PostgreSQL database.",
			RequiresConfirmation: true,
		}),
	)

	// Create the PostgreSQL app
	pg, err := app.New(
		app.Config{
			Name: "postgres",
		},
		app.WithResource(pgdb),
	)
	if err != nil {
		panic(err)
	}

	// Create and configure the agent
	agentInstance := agent.New(agent.Config{
		APIKey:     os.Getenv("TEMPEST_API_KEY"),
		ServerAddr: ":8080",
		QueueOptions: agent.QueueOptions{
			NumWorkers:       5,
			NumResultWorkers: 2,
			PollTimeout:      100 * time.Millisecond,
		},
	})

	// Register the PostgreSQL app with the agent
	// The second parameter defines which versions of the app the agent supports
	agentInstance.RegisterApp(pg, []string{"v1"})

	// Start the agent in a goroutine
	go func() {
		fmt.Println("Starting PostgreSQL agent on port 8080...")
		if err := agentInstance.Start(); err != nil {
			log.Fatalf("Failed to start agent: %v", err)
		}
	}()

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// Gracefully shutdown the agent
	fmt.Println("Shutting down PostgreSQL agent...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := agentInstance.Stop(ctx); err != nil {
		log.Fatalf("Failed to stop agent: %v", err)
	}

	fmt.Println("PostgreSQL agent stopped")
}
