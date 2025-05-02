package main

import (
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/tempestdx/sdk-go/agent"
	"github.com/tempestdx/sdk-go/app"
	"github.com/tempestdx/sdk-go/examples/gitlab/resources"
)

func main() {
	// Create the GitLab resource definition
	gitlabRepoResource, err := resources.NewRepositoryResource()
	if err != nil {
		log.Fatalf("Failed to create GitLab repository resource: %v", err)
	}

	gitlabApp, err := app.New(app.Config{
		ID:   "gitlab",
		Name: "GitLab",
	},
		app.WithResource(gitlabRepoResource),
		app.WithInterface(app.GitRepositoryInterface), // Indicate this app implements the git repository interface
	)
	if err != nil {
		panic(err)
	}

	// Create a new Tempest agent
	agt, err := agent.New(
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
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Register the GitLab app with the agent
	err = agt.RegisterApp(gitlabApp)
	if err != nil {
		log.Fatalf("Failed to register GitLab app: %v", err)
	}

	// Run the agent
	log.Println("Starting GitLab example agent...")
	if err := agt.Run(); err != nil {
		log.Fatalf("Agent terminated with error: %v", err)
	}
}
