package main

import (
	"context"
	"fmt"

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
	// ...
	return &resource.OperationResponse{
		Resource: req.Resource,
	}, nil
}

func main() {
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

	pg, err := app.New(
		app.Config{
			Name: "postgres",
		},
		app.WithResource(pgdb),
	)

	if err != nil {
		panic(err)
	}

	json, err := pg.JSON()
	if err != nil {
		panic(err)
	}

	fmt.Println(string(json))
}
