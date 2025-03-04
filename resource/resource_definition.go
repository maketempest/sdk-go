package resource

import (
	"encoding/json"
	"errors"
	"maps"

	"github.com/tempestdx/sdk-go/jsonschema"
)

// resourceStatefulMetadata contains the published state of a resource
type resourceStatefulMetadata struct {
	version        string
	published      bool
	publishedID    string
	organizationID string
	recordID       string
	appID          string
}

type ResourceDefinition struct {
	// The display name of your resouce
	displayName string
	// The unique ID of your resource, if not specified, your display name will be slugified
	uniqueID string

	lifecycleStage       LifecycleStage
	categories           []Category
	links                []Link
	properties           *jsonschema.Schema
	instructionsMarkdown string

	operations  map[string]*Operation
	healthCheck HealthCheckFunc

	publishedState *resourceStatefulMetadata
}

func (r *ResourceDefinition) UniqueID() string {
	return r.uniqueID
}

func (r *ResourceDefinition) DisplayName() string {
	return r.displayName
}

func (r *ResourceDefinition) Operations() map[string]*Operation {
	// returns a copy of the operations
	operationsCopy := make(map[string]*Operation, len(r.operations))
	maps.Copy(operationsCopy, r.operations)
	return operationsCopy
}

// Config represents the configuration for creating a new resource
type DefinitionConfig struct {
	DisplayName    string
	UniqueID       string
	LifecycleStage LifecycleStage
	Properties     *jsonschema.Schema
}

// New creates a new V2 resource
func NewDefinition(config DefinitionConfig, opts ...resourceDefinitionOption) (*ResourceDefinition, error) {
	r := &ResourceDefinition{
		displayName:    config.DisplayName,
		uniqueID:       config.UniqueID,
		lifecycleStage: config.LifecycleStage,
		properties:     config.Properties,
		operations:     make(map[string]*Operation),
	}

	if config.DisplayName == "" {
		return nil, errors.New("display name is required")
	}

	if config.UniqueID == "" {
		return nil, errors.New("unique id is required")
	}

	// validate links
	for _, link := range r.links {
		link.setDefault()
		if !link.isValid() {
			return nil, errors.New("invalid link")
		}
	}

	// validate categories
	for _, category := range r.categories {
		if !isValidCategory(category) {
			return nil, errors.New("invalid category")
		}
	}

	// validate properties
	if r.properties == nil {
		return nil, errors.New("properties are required")
	}

	// parse properties
	if _, err := jsonschema.ParseSchema(r.properties.Raw); err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(r)
	}

	// validate categories
	for _, category := range r.categories {
		if !isValidCategory(category) {
			return nil, errors.New("invalid category")
		}
	}

	return r, nil
}

type resourceDefinitionOption func(*ResourceDefinition)

func WithDefaultLinks(links ...Link) resourceDefinitionOption {
	// Validate links
	return func(r *ResourceDefinition) {
		r.links = links
	}
}

func WithInstructions(markdown string) resourceDefinitionOption {
	return func(r *ResourceDefinition) {
		r.instructionsMarkdown = markdown
	}
}

func WithHealthCheck(fn HealthCheckFunc) resourceDefinitionOption {
	return func(r *ResourceDefinition) {
		r.healthCheck = fn
	}
}

func WithCategories(categories ...Category) resourceDefinitionOption {
	return func(r *ResourceDefinition) {
		r.categories = categories
	}
}

func (r *ResourceDefinition) RegisterOperation(name string, fn OperationFunc, opts ...operationOption) *ResourceDefinition {
	op := NewOperation(name, fn, opts...)
	r.operations[name] = op
	return r
}

// JSON returns the JSON representation of the ResourceDefinition
func (r *ResourceDefinition) JSON() ([]byte, error) {
	data := map[string]any{
		"displayName":    r.displayName,
		"uniqueID":       r.uniqueID,
		"lifecycleStage": r.lifecycleStage,
	}

	// Add categories if any
	if len(r.categories) > 0 {
		categories := make([]string, len(r.categories))
		for i, category := range r.categories {
			categories[i] = string(category)
		}
		data["categories"] = categories
	}

	// Add links if any
	if len(r.links) > 0 {
		data["links"] = r.links
	}

	// Add properties schema if any
	if r.properties != nil {
		propertiesSchema, err := r.properties.Raw.MarshalJSON()
		if err != nil {
			return nil, err
		}
		data["properties"] = json.RawMessage(propertiesSchema)
	}

	// Add instructions if any
	if r.instructionsMarkdown != "" {
		data["instructionsMarkdown"] = r.instructionsMarkdown
	}

	// Add health check flag if defined
	if r.healthCheck != nil {
		data["healthcheckEnabled"] = true
	}

	// Add operations if any
	if len(r.operations) > 0 {
		ops := make(map[string]json.RawMessage)
		for name, op := range r.operations {
			opJSON, err := op.JSON()
			if err != nil {
				return nil, err
			}
			ops[name] = opJSON
		}
		data["operations"] = ops
	}

	return json.Marshal(data)
}

// GetOperation returns the operation with the given name.
func (r *ResourceDefinition) GetOperation(name string) (*Operation, bool) {
	op, ok := r.Operations()[name]
	return op, ok
}

// PropertiesSchema returns the properties schema for the resource
func (r *ResourceDefinition) PropertiesSchema() *jsonschema.Schema {
	return r.properties
}

// LifecycleStage returns the lifecycle stage of the resource
func (r *ResourceDefinition) LifecycleStage() LifecycleStage {
	return r.lifecycleStage
}

// Categories returns the categories of the resource
func (r *ResourceDefinition) Categories() []Category {
	return r.categories
}

// Links returns the links associated with the resource
func (r *ResourceDefinition) Links() []Link {
	return r.links
}

// InstructionsMarkdown returns the instructions for the resource
func (r *ResourceDefinition) InstructionsMarkdown() string {
	return r.instructionsMarkdown
}

// HasHealthCheck returns true if the resource has a health check
func (r *ResourceDefinition) HasHealthCheck() bool {
	return r.healthCheck != nil
}
