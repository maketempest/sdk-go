package app

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/tempestdx/sdk-go/resource"
)

type App struct {
	Name         string
	resourceDefs map[string]*resource.ResourceDefinition
}

func (a *App) ResourceDefinitions() map[string]*resource.ResourceDefinition {
	// returns a copy of the resource definitions
	resourceDefsCopy := make(map[string]*resource.ResourceDefinition, len(a.resourceDefs))
	maps.Copy(resourceDefsCopy, a.resourceDefs)
	return resourceDefsCopy
}

type OptFunc func(*App) error

type Config struct {
	Name string
}

func New(config Config, opts ...OptFunc) (*App, error) {
	a := &App{
		Name:         config.Name,
		resourceDefs: make(map[string]*resource.ResourceDefinition),
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, err
		}
	}

	if a.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}

	return a, nil
}

func WithResource(r *resource.ResourceDefinition) OptFunc {
	return func(a *App) error {
		// Check if the resource name is already in use
		if _, ok := a.resourceDefs[r.UniqueID()]; ok {
			return fmt.Errorf("resource %s already exists", r.UniqueID())
		}
		a.resourceDefs[r.UniqueID()] = r

		return nil
	}
}

func (a *App) Resources() map[string]*resource.ResourceDefinition {
	// Create a copy of the resources map to prevent external modification
	resourcesCopy := make(map[string]*resource.ResourceDefinition, len(a.resourceDefs))
	maps.Copy(resourcesCopy, a.resourceDefs)
	return resourcesCopy
}

// JSON returns the JSON representation of the App
func (a *App) JSON() ([]byte, error) {
	data := map[string]any{
		"name": a.Name,
	}

	// Add resource definitions if any
	if len(a.resourceDefs) > 0 {
		resourceDefs := make(map[string]json.RawMessage)
		for id, rd := range a.resourceDefs {
			rdJSON, err := rd.JSON()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal resource definition %s: %w", id, err)
			}
			resourceDefs[id] = rdJSON
		}
		data["resourceDefinitions"] = resourceDefs
	}

	return json.Marshal(data)
}

// GetResourceDefinition returns the resource definition with the given ID.
// The ID is expected to be the UniqueID of the resource definition.
func (a *App) GetResourceDefinition(resourceID string) (*resource.ResourceDefinition, bool) {
	for _, rd := range a.ResourceDefinitions() {
		if rd.UniqueID() == resourceID {
			return rd, true
		}
	}
	return nil, false
}
