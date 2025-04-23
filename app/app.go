package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/resource"
)

type App struct {
	Name           string
	resourceDefs   map[string]*resource.Definition
	datasourceDefs map[string]*datasource.Definition
	// Names of credential providers supported by this app
	credentialProviders []string
	// List of implemented plug and play interfaces
	implementedInterfaces []InterfaceType
}

func (a *App) ResourceDefinitions() map[string]*resource.Definition {
	// returns a copy of the resource definitions
	resourceDefsCopy := make(map[string]*resource.Definition, len(a.resourceDefs))
	maps.Copy(resourceDefsCopy, a.resourceDefs)
	return resourceDefsCopy
}

func (a *App) DataSourceDefinitions() map[string]*datasource.Definition {
	// returns a copy of the datasource definitions
	datasourceDefsCopy := make(map[string]*datasource.Definition, len(a.datasourceDefs))
	maps.Copy(datasourceDefsCopy, a.datasourceDefs)
	return datasourceDefsCopy
}

type OptFunc func(*App) error

type Config struct {
	Name string
}

func New(config Config, opts ...OptFunc) (*App, error) {
	a := &App{
		Name:           config.Name,
		resourceDefs:   make(map[string]*resource.Definition),
		datasourceDefs: make(map[string]*datasource.Definition),
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

func WithResource(r *resource.Definition) OptFunc {
	return func(a *App) error {
		// Check if the resource name is already in use
		if _, ok := a.resourceDefs[r.UniqueID()]; ok {
			return fmt.Errorf("resource %s already exists", r.UniqueID())
		}
		a.resourceDefs[r.UniqueID()] = r

		return nil
	}
}

func WithDataSource(d *datasource.Definition) OptFunc {
	return func(a *App) error {
		// Check if the datasource name is already in use
		if _, ok := a.datasourceDefs[d.UniqueID()]; ok {
			return fmt.Errorf("datasource %s already exists", d.UniqueID())
		}
		a.datasourceDefs[d.UniqueID()] = d

		return nil
	}
}

func WithCredentials(credentials []string) OptFunc {
	return func(a *App) error {
		a.credentialProviders = credentials
		return nil
	}
}

func WithInterface(iface InterfaceType) OptFunc {
	return func(a *App) error {
		if !iface.IsValid() {
			return fmt.Errorf("invalid interface type: %s", iface)
		}
		if slices.Contains(a.implementedInterfaces, iface) {
			return nil // Already added
		}
		a.implementedInterfaces = append(a.implementedInterfaces, iface)
		return nil
	}
}

func (a *App) CredentialProviders() []string {
	return a.credentialProviders
}

func (a *App) Resources() map[string]*resource.Definition {
	// Create a copy of the resources map to prevent external modification
	resourcesCopy := make(map[string]*resource.Definition, len(a.resourceDefs))
	maps.Copy(resourcesCopy, a.resourceDefs)
	return resourcesCopy
}

// ImplementedInterfaces returns a copy of the implemented plug and play interfaces.
func (a *App) ImplementedInterfaces() []InterfaceType {
	interfacesCopy := make([]InterfaceType, len(a.implementedInterfaces))
	copy(interfacesCopy, a.implementedInterfaces)
	return interfacesCopy
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

	// Add datasource definitions if any
	if len(a.datasourceDefs) > 0 {
		datasourceDefs := make(map[string]json.RawMessage)
		for id, dd := range a.datasourceDefs {
			ddJSON, err := dd.JSON()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal datasource definition %s: %w", id, err)
			}
			datasourceDefs[id] = ddJSON
		}
		data["datasourceDefinitions"] = datasourceDefs
	}

	// Add implemented interfaces if any
	if len(a.implementedInterfaces) > 0 {
		// Convert InterfaceType slice to string slice for JSON
		interfaceStrings := make([]string, len(a.implementedInterfaces))
		for i, iface := range a.implementedInterfaces {
			interfaceStrings[i] = string(iface)
		}
		data["implementedInterfaces"] = interfaceStrings
	}

	return json.Marshal(data)
}

// GetResourceDefinition returns the resource definition with the given ID.
// The ID is expected to be the UniqueID of the resource definition.
func (a *App) GetResourceDefinition(resourceID string) (*resource.Definition, bool) {
	for _, rd := range a.ResourceDefinitions() {
		if rd.UniqueID() == resourceID {
			return rd, true
		}
	}
	return nil, false
}

// GetDataSourceDefinition returns the datasource definition with the given ID.
// The ID is expected to be the UniqueID of the datasource definition.
func (a *App) GetDataSourceDefinition(datasourceID string) (*datasource.Definition, bool) {
	for _, dd := range a.DataSourceDefinitions() {
		if dd.UniqueID() == datasourceID {
			return dd, true
		}
	}
	return nil, false
}
