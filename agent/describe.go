package agent

import (
	"encoding/json"
	"net/http"
)

// OperationDescription represents an operation in the describe API response
type OperationDescription struct {
	Name             string                   `json:"name"`
	Args             json.RawMessage          `json:"args,omitempty"`
	ActionConfig     map[string]interface{}   `json:"action_config,omitempty"`
	CanonicalBinding []map[string]interface{} `json:"canonical_binding,omitempty"`
}

// ResourceDescription represents a resource in the describe API response
type ResourceDescription struct {
	DisplayName          string                          `json:"display_name"`
	UniqueID             string                          `json:"unique_id"`
	LifecycleStage       string                          `json:"lifecycle_stage"`
	PropertiesSchema     json.RawMessage                 `json:"properties_schema,omitempty"`
	Categories           []string                        `json:"categories,omitempty"`
	Links                []map[string]interface{}        `json:"links,omitempty"`
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
	a.mux.HandleFunc("/describe", func(w http.ResponseWriter, r *http.Request) {
		a.logger.Debug("Handling describe request",
			"method", r.Method,
			"remote_addr", r.RemoteAddr)

		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Prepare the response structure
		response := make(map[string]AppDescription)

		// Gather information from all registered apps
		for appName, appConfig := range a.apps {
			appDesc := AppDescription{
				Name:              appName,
				SupportedVersions: appConfig.SupportedVersions,
				Resources:         make(map[string]ResourceDescription),
			}

			// Get resource definitions
			for _, resourceDef := range appConfig.ResourceDefinitions() {
				resDesc := ResourceDescription{
					DisplayName: resourceDef.DisplayName(),
					UniqueID:    resourceDef.UniqueID(),
					Operations:  make(map[string]OperationDescription),
				}

				// Collect operations
				for opName, operation := range resourceDef.Operations() {
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
						canonicalBindings := make([]map[string]interface{}, 0, len(canonOps))
						for _, op := range canonOps {
							canonicalBindings = append(canonicalBindings, map[string]interface{}{
								"type":        op.Type.String(),
								"priority":    op.Priority,
								"concurrency": op.Concurrency,
							})
						}
						opDesc.CanonicalBinding = canonicalBindings
					}

					// Get action config if available
					if actionConfig := operation.ActionConfig(); actionConfig != nil {
						opDesc.ActionConfig = map[string]any{
							"title":                 actionConfig.Title,
							"description":           actionConfig.Description,
							"requires_confirmation": actionConfig.RequiresConfirmation,
						}
					}

					resDesc.Operations[opName] = opDesc
				}

				// Get resource properties schema if available
				if props := resourceDef.PropertiesSchema(); props != nil {
					propsJSON, _ := props.Raw.MarshalJSON()
					resDesc.PropertiesSchema = propsJSON
				}

				// Get lifecycle stage
				resDesc.LifecycleStage = resourceDef.LifecycleStage().String()

				// Get categories
				if cats := resourceDef.Categories(); len(cats) > 0 {
					categories := make([]string, len(cats))
					for i, cat := range cats {
						categories[i] = string(cat)
					}
					resDesc.Categories = categories
				}

				// Get links
				if links := resourceDef.Links(); len(links) > 0 {
					linksMaps := make([]map[string]interface{}, len(links))
					for i, link := range links {
						linksMaps[i] = map[string]interface{}{
							"title":    link.Title,
							"url":      link.URL,
							"type":     string(link.Type),
							"category": string(link.Category),
						}
					}
					resDesc.Links = linksMaps
				}

				// Get instructions
				resDesc.InstructionsMarkdown = resourceDef.InstructionsMarkdown()

				// Check if healthcheck is enabled
				resDesc.HealthcheckEnabled = resourceDef.HasHealthCheck()

				// Add the resource description to the app
				appDesc.Resources[resourceDef.UniqueID()] = resDesc
			}

			// Add canonical bindings to the app description with proper formatting
			if len(appConfig.CanonicalBindings) > 0 {
				// Create a JSON-friendly structure with string keys for canonical types
				jsonBindings := make(map[string]map[string][]jsonCanonicalBinding)

				for resName, typeBindings := range appConfig.CanonicalBindings {
					jsonBindings[string(resName)] = make(map[string][]jsonCanonicalBinding)

					for canonType, bindings := range typeBindings {
						// Convert canonical type enum to string
						canonTypeStr := canonType.String()

						// Convert bindings to JSON-friendly format
						jsonBindingsList := make([]jsonCanonicalBinding, 0, len(bindings))
						for _, binding := range bindings {
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

				// Convert to JSON
				canonicalBindingsJSON, err := json.Marshal(jsonBindings)
				if err != nil {
					a.logger.Error("Failed to marshal canonical bindings", "error", err)
				} else {
					appDesc.CanonicalBindings = canonicalBindingsJSON
				}
			}

			// Also add canonical routes information
			routes := appConfig.generateCanonicalRoutes()
			if len(routes) > 0 {
				routesJSON, err := json.Marshal(routes)
				if err != nil {
					a.logger.Error("Failed to marshal canonical routes", "error", err)
				} else {
					// You might need to add a field for routes to AppDescription
					// For now we can add it to the existing canonical bindings data
					appDesc.Routes = routesJSON // This would require adding a Routes field to AppDescription
				}
			}

			// Add the app description to the response
			response[appName] = appDesc
		}

		// Set content type and return the JSON response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			a.logger.Error("Failed to encode describe response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})
}
