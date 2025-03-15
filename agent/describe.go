package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/tempestdx/sdk-go/resource"
)

// OperationDescription represents an operation in the describe API response
type OperationDescription struct {
	Name             string                   `json:"name"`
	Args             json.RawMessage          `json:"args,omitempty"`
	ActionConfig     map[string]any   `json:"action_config,omitempty"`
	CanonicalBinding []map[string]any `json:"canonical_binding,omitempty"`
}

// ResourceDescription represents a resource in the describe API response
type ResourceDescription struct {
	DisplayName          string                          `json:"display_name"`
	UniqueID             string                          `json:"unique_id"`
	LifecycleStage       string                          `json:"lifecycle_stage"`
	PropertiesSchema     json.RawMessage                 `json:"properties_schema,omitempty"`
	Categories           []string                        `json:"categories,omitempty"`
	Links                []map[string]any        `json:"links,omitempty"`
	InstructionsMarkdown string                          `json:"instructions_markdown,omitempty"`
	Operations           map[string]OperationDescription `json:"operations,omitempty"`
	HealthcheckEnabled   bool                            `json:"healthcheck_enabled,omitempty"`
}

// AppDescription represents an app in the describe API response
type AppDescription struct {
	Name              string                         `json:"name"`
	SupportedVersions []string                       `json:"supported_versions"`
	Resources         map[string]ResourceDescription `json:"resources"`
	CanonicalBindings json.RawMessage                `json:"canonical_bindings,omitempty"`
	Routes            json.RawMessage                `json:"routes,omitempty"`
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

func (a *agent) buildDescribeResponse() map[string]AppDescription {
	response := make(map[string]AppDescription)

	// Gather information from all registered apps
	for appName, appConfig := range a.apps {
		appDesc := buildAppDescription(appName, appConfig, a.logger)
		response[appName] = appDesc
	}

	return response
}

// Helper functions that don't need agent access

func buildAppDescription(appName string, appConfig *appConfig, logger *slog.Logger) AppDescription {
	appDesc := AppDescription{
		Name:              appName,
		SupportedVersions: appConfig.SupportedVersions,
		Resources:         make(map[string]ResourceDescription),
	}

	// Get resource definitions
	for _, resourceDef := range appConfig.ResourceDefinitions() {
		resDesc := buildResourceDescription(resourceDef)
		appDesc.Resources[resourceDef.UniqueID()] = resDesc
	}

	// Add canonical bindings
	if len(appConfig.ResourceCanonicalMap) > 0 {
		appDesc.CanonicalBindings = marshalCanonicalBindings(appConfig.ResourceCanonicalMap, logger)
	}

	// Add canonical routes information
	routes := appConfig.generateCanonicalRoutes()
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
			"title":                 actionConfig.Title,
			"description":           actionConfig.Description,
			"requires_confirmation": actionConfig.RequiresConfirmation,
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
