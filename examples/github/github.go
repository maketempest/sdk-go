package main

import (
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/tempestdx/sdk-go/agent"
	"github.com/tempestdx/sdk-go/app"
	"github.com/tempestdx/sdk-go/examples/github/resources"
)

func main() {
	// Create the GitHub resource definition
	githubRepoResource, err := resources.NewRepositoryResource()
	if err != nil {
		log.Fatalf("Failed to create GitHub repository resource: %v", err)
	}

	githubApp, err := app.New(app.Config{
		ID:   "github",
		Name: "GitHub",
	},
		app.WithResource(githubRepoResource),
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

	// Register the GitHub app with the agent
	err = agt.RegisterApp(githubApp)
	if err != nil {
		log.Fatalf("Failed to register GitHub app: %v", err)
	}

	// Run the agent
	log.Println("Starting GitHub example agent...")
	if err := agt.Run(); err != nil {
		log.Fatalf("Agent terminated with error: %v", err)
	}
}
