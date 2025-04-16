package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"maps"

	"github.com/tempestdx/sdk-go/credential"
	"github.com/tempestdx/sdk-go/datasource"
	"github.com/tempestdx/sdk-go/resource"
)

// OperationDescription represents an operation in the describe API response
type OperationDescription struct {
	Name             string           `json:"name"`
	Args             json.RawMessage  `json:"args,omitempty"`
	ActionConfig     map[string]any   `json:"actionConfig,omitempty"`
	CanonicalBinding []map[string]any `json:"canonicalBinding,omitempty"`
}

type DataSourceDescription struct {
	Name                 string          `json:"name"`
	InputSchema          json.RawMessage `json:"inputSchema"`
	OutputSchema         json.RawMessage `json:"outputSchema"`
	InstructionsMarkdown string          `json:"instructionsMarkdown"`
}

// CredentialDescription represents a credential in the describe API response
type CredentialDescription struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

// ResourceDescription represents a resource in the describe API response
type ResourceDescription struct {
	DisplayName          string                          `json:"displayName"`
	UniqueID             string                          `json:"uniqueId"`
	LifecycleStage       string                          `json:"lifecycleStage"`
	PropertiesSchema     json.RawMessage                 `json:"propertiesSchema,omitempty"`
	Categories           []string                        `json:"categories,omitempty"`
	Links                []map[string]any                `json:"links,omitempty"`
	Credentials          []string                        `json:"credentials,omitempty"`
	InstructionsMarkdown string                          `json:"instructionsMarkdown,omitempty"`
	Operations           map[string]OperationDescription `json:"operations,omitempty"`
	HealthcheckEnabled   bool                            `json:"healthcheckEnabled,omitempty"`
}

// AppDescription represents an app in the describe API response
type AppDescription struct {
	Name                string                           `json:"name"`
	Version             string                           `json:"version"`
	Resources           map[string]ResourceDescription   `json:"resources"`
	Datasources         map[string]DataSourceDescription `json:"datasources"`
	CredentialProviders []string                         `json:"credentialProviders,omitempty"`
	CanonicalBindings   json.RawMessage                  `json:"canonicalBindings,omitempty"`
	Routes              json.RawMessage                  `json:"routes,omitempty"`
}

type describeResponse struct {
	Apps                map[string]AppDescription        `json:"apps"`
	CredentialProviders map[string]CredentialDescription `json:"credentialProviders"`
}

// registerDescribeHandler registers a handler to describe all registered resources and operations
func (a *agent) registerDescribeHandler() {
	a.logger.Debug("Registering describe handler", "path", "/describe")

	// Register the handler for the describe route
	a.mux.HandleFunc("/describe", a.handleDescribeRequest)
}

func (a *agent) handleDescribeRequest(w http.ResponseWriter, r *http.Request) {
	a.logger.Debug("Handling describe request",
		"method", r.Method,
		"remote_addr", r.RemoteAddr)

	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := a.buildDescribeResponse()

	// Set content type and return the JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		a.logger.Error("Failed to encode describe response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (a *agent) buildDescribeResponse() describeResponse {
	response := describeResponse{
		Apps:                map[string]AppDescription{},
		CredentialProviders: map[string]CredentialDescription{},
	}

	// Gather information from all registered apps
	for appName, appConfig := range a.apps {
		routes := make(map[string]string)
		maps.Copy(routes, appConfig.generateCanonicalRoutes())
		maps.Copy(routes, appConfig.generateOperationRoutes())
		maps.Copy(routes, appConfig.generateDataSourceRoutes())

		appDesc := newAppDescription(appConfig, a.logger).withRoutes(routes, a.logger)
		response.Apps[appName] = appDesc
	}

	// Gather information from all registered credential providers
	for providerName, credentialProvider := range a.credentialProviders {
		response.CredentialProviders[providerName] = newCredentialDescription(credentialProvider)
	}

	return response
}

func newCredentialDescription(credentials *credential.CredentialProvider) CredentialDescription {
	return CredentialDescription{
		Name:   credentials.Name,
		Type:   string(credentials.Type),
		Schema: credentials.Schema.Raw,
	}
}

// Helper functions that don't need agent access
func newAppDescription(appConfig *appConfig, logger *slog.Logger) AppDescription {
	appDesc := AppDescription{
		Name:        appConfig.Name,
		Resources:   make(map[string]ResourceDescription),
		Datasources: make(map[string]DataSourceDescription),
	}

	if appConfig.Version != "" {
		appDesc = appDesc.withVersion(appConfig.Version)
	}

	if len(appConfig.CredentialProviders()) > 0 {
		appDesc.CredentialProviders = appConfig.CredentialProviders()
	}

	for _, resourceDef := range appConfig.ResourceDefinitions() {
		appDesc.Resources[resourceDef.UniqueID()] = buildResourceDescription(resourceDef)
	}

	for _, datasourceDef := range appConfig.DataSourceDefinitions() {
		appDesc.Datasources[datasourceDef.UniqueID()] = buildDataSourceDescription(datasourceDef)
	}

	if len(appConfig.ResourceCanonicalMap) > 0 {
		appDesc.CanonicalBindings = marshalCanonicalBindings(appConfig.ResourceCanonicalMap, logger)
	}

	appConfig.generateCanonicalRoutes()

	return appDesc
}

func buildDataSourceDescription(datasourceDef *datasource.Definition) DataSourceDescription {
	return DataSourceDescription{
		Name:                 datasourceDef.UniqueID(),
		InputSchema:          datasourceDef.InputSchema().Raw,
		OutputSchema:         datasourceDef.OutputSchema().Raw,
		InstructionsMarkdown: datasourceDef.InstructionsMarkdown(),
	}
}

// withVersion adds version information to the app description
func (appDesc AppDescription) withVersion(version string) AppDescription {
	appDesc.Version = version

	return appDesc
}

func (appDesc AppDescription) withRoutes(routes map[string]string, logger *slog.Logger) AppDescription {
	if len(routes) > 0 {
		appDesc.Routes = marshalData(routes, logger)
	}

	return appDesc
}

func buildResourceDescription(resourceDef *resource.Definition) ResourceDescription {
	resDesc := ResourceDescription{
		DisplayName:          resourceDef.DisplayName(),
		UniqueID:             resourceDef.UniqueID(),
		Operations:           make(map[string]OperationDescription),
		LifecycleStage:       resourceDef.LifecycleStage().String(),
		HealthcheckEnabled:   resourceDef.HasHealthCheck(),
		InstructionsMarkdown: resourceDef.InstructionsMarkdown(),
	}

	// Process operations
	for opName, operation := range resourceDef.Operations() {
		resDesc.Operations[opName] = buildOperationDescription(opName, operation)
	}

	// Get resource properties schema if available
	if props := resourceDef.PropertiesSchema(); props != nil {
		propsJSON, _ := props.Raw.MarshalJSON()
		resDesc.PropertiesSchema = propsJSON
	}

	// Get categories
	if cats := resourceDef.Categories(); len(cats) > 0 {
		resDesc.Categories = convertCategories(cats)
	}

	// Get links
	if links := resourceDef.Links(); len(links) > 0 {
		resDesc.Links = convertLinks(links)
	}

	// Get credentials
	if creds := resourceDef.Credentials(); len(creds) > 0 {
		resDesc.Credentials = creds
	}

	return resDesc
}

func buildOperationDescription(opName string, operation *resource.Operation) OperationDescription {
	opDesc := OperationDescription{
		Name: opName,
	}

	// Get operation schema if available
	if operation.Args() != nil {
		argsJSON, _ := operation.Args().Raw.MarshalJSON()
		opDesc.Args = argsJSON
	}

	// Get canonical operations
	if canonOps := operation.CanonicalOperations(); len(canonOps) > 0 {
		opDesc.CanonicalBinding = convertCanonicalOperations(canonOps)
	}

	// Get action config if available
	if actionConfig := operation.ActionConfig(); actionConfig != nil {
		opDesc.ActionConfig = map[string]any{
			"title":                actionConfig.Title,
			"description":          actionConfig.Description,
			"requiresConfirmation": actionConfig.RequiresConfirmation,
		}
	}

	return opDesc
}

func convertCanonicalOperations(canonOps []resource.CanonicalOperation) []map[string]any {
	canonicalBindings := make([]map[string]any, 0, len(canonOps))
	for _, op := range canonOps {
		canonicalBindings = append(canonicalBindings, map[string]any{
			"type":        op.Type.String(),
			"priority":    op.Priority,
			"concurrency": op.Concurrency,
		})
	}
	return canonicalBindings
}

func convertCategories(cats []resource.Category) []string {
	categories := make([]string, len(cats))
	for i, cat := range cats {
		categories[i] = string(cat)
	}
	return categories
}

func convertLinks(links []resource.Link) []map[string]any {
	linksMaps := make([]map[string]any, len(links))
	for i, link := range links {
		linksMaps[i] = map[string]any{
			"title":    link.Title,
			"url":      link.URL,
			"type":     string(link.Type),
			"category": string(link.Category),
		}
	}
	return linksMaps
}

func marshalCanonicalBindings(bindings resourceCanonicalBindings, logger *slog.Logger) json.RawMessage {
	// Create a JSON-friendly structure with string keys for canonical types
	jsonBindings := make(map[string]map[string][]jsonCanonicalBinding)

	for resName, typeBindings := range bindings {
		jsonBindings[string(resName)] = make(map[string][]jsonCanonicalBinding)

		for canonType, bindingsList := range typeBindings {
			// Convert canonical type enum to string
			canonTypeStr := canonType.String()

			// Convert bindings to JSON-friendly format
			jsonBindingsList := make([]jsonCanonicalBinding, 0, len(bindingsList))
			for _, binding := range bindingsList {
				jsonOps := make([]jsonOperationMetadata, 0, len(binding.Operations))
				for _, op := range binding.Operations {
					jsonOps = append(jsonOps, jsonOperationMetadata{
						Name:        op.Name,
						Priority:    op.Priority,
						Concurrency: op.Concurrency,
					})
				}
				jsonBindingsList = append(jsonBindingsList, jsonCanonicalBinding{
					Operations: jsonOps,
				})
			}

			jsonBindings[string(resName)][canonTypeStr] = jsonBindingsList
		}
	}

	return marshalData(jsonBindings, logger)
}

// Generic helper for marshaling data to JSON
func marshalData(data any, logger *slog.Logger) json.RawMessage {
	bytes, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal data", "error", err)
		return nil
	}
	return bytes
}
