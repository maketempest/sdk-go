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

// First create operation with priority 100
func pgdbCreatePrepare(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	fmt.Println("[Priority 100] Preparing to create PostgreSQL database...")
	fmt.Println(req.Args["name"])
	return &resource.OperationResponse{
		Resource: req.Resource,
	}, nil
}

// Second create operation with priority 100 (will run after the first one due to registration order)
func pgdbCreateValidate(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	fmt.Println("[Priority 100] Validating PostgreSQL database parameters...")
	fmt.Println(req.Args["name"])

	return &resource.OperationResponse{
		Resource: req.Resource,
	}, nil
}

// Main create operation with priority 200 (will run last)
func pgdbCreate(ctx context.Context, req *resource.OperationRequest) (*resource.OperationResponse, error) {
	fmt.Println("[Priority 200] Creating PostgreSQL database...")

	fmt.Println(req.Args["name"])

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

	// Register multiple operations for create canonical type with different priorities

	// Preparation step - Priority 100
	pgdb.RegisterOperation(
		"create_prep",
		pgdbCreatePrepare,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(pgdbCreateArgsSchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Create,
			Priority:    100,
			Concurrency: 1,
		}),
	)

	// Validation step - Priority 100 (will run after preparation due to registration order)
	pgdb.RegisterOperation(
		"create_validate",
		pgdbCreateValidate,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(pgdbCreateArgsSchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Create,
			Priority:    100,
			Concurrency: 1,
		}),
	)

	// Main creation step - Priority 200 (will run last)
	pgdb.RegisterOperation(
		"create_db",
		pgdbCreate,
		resource.Op.WithArgs(jsonschema.MustParseSchema([]byte(pgdbCreateArgsSchema))),
		resource.Op.On(resource.CanonicalOperation{
			Type:        resource.Create,
			Priority:    200,
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

	// Create and configure the agent using the new functional options pattern
	// This will automatically use environment variables if set, or use these explicitly provided options
	agentInstance := agent.New(
		// Specify server address explicitly
		agent.WithServerAddr(":8080"),

		// Set worker counts
		agent.WithWorkers(5),
		agent.WithResultWorkers(2),

		// Set poll timeout
		agent.WithPollTimeout(100*time.Millisecond),

		// Configure logging to use text format instead of default JSON
		agent.WithLogger(slog.New(
			slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}),
		)),

		// Set custom stop options
		agent.WithStopOptions(agent.StopOptions{
			NoTimeout: true,
		}),
	)

	// To see how environment variables can be used, you could set:
	// export TEMPEST_API_KEY="your-api-key"
	// export TEMPEST_SERVER_ADDR=":8080"
	// export TEMPEST_NUM_WORKERS="5"
	// export TEMPEST_NUM_RESULT_WORKERS="2"
	// export TEMPEST_POLL_TIMEOUT="100ms"
	// export TEMPEST_LOG_LEVEL="info"
	// export TEMPEST_LOG_FORMAT="text"
	//
	// And then simply use: agentInstance := agent.New()

	// Register the PostgreSQL app with the agent
	// The second parameter defines which versions of the app the agent supports
	agentInstance.RegisterApp(pg, []string{"v1"})

	fmt.Println("Starting PostgreSQL agent on port 8080...")
	if err := agentInstance.Run(); err != nil {
		log.Fatalf("Failed to run agent: %v", err)
	}

	fmt.Println("PostgreSQL agent stopped")

	// Later, when stopping:
	ctx := context.Background()
	err = agentInstance.Stop(ctx) // Will use the configured stop options
	if err != nil {
		log.Fatalf("Failed to stop agent: %v", err)
	}
}
