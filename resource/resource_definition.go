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

type Definition struct {
	// The display name of your resouce
	displayName string
	// The unique ID of your resource, if not specified, your display name will be slugified
	uniqueID string

	lifecycleStage       LifecycleStage
	categories           []Category
	links                []Link
	credentials          []Credential
	properties           *jsonschema.Schema
	instructionsMarkdown string

	operations  map[string]*Operation
	healthCheck HealthCheckFunc

	publishedState *resourceStatefulMetadata
}

func (r *Definition) UniqueID() string {
	return r.uniqueID
}

func (r *Definition) DisplayName() string {
	return r.displayName
}

func (r *Definition) Operations() map[string]*Operation {
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
func NewDefinition(config DefinitionConfig, opts ...resourceDefinitionOption) (*Definition, error) {
	r := &Definition{
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

	// validate credentials
	for _, credential := range r.credentials {
		credential.setDefault()
		if !credential.isValid() {
			return nil, errors.New("invalid credential")
		}
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

type resourceDefinitionOption func(*Definition)

func WithDefaultLinks(links ...Link) resourceDefinitionOption {
	return func(r *Definition) {
		r.links = links
	}
}

func WithInstructions(markdown string) resourceDefinitionOption {
	return func(r *Definition) {
		r.instructionsMarkdown = markdown
	}
}

func WithHealthCheck(fn HealthCheckFunc) resourceDefinitionOption {
	return func(r *Definition) {
		r.healthCheck = fn
	}
}

func WithCategories(categories ...Category) resourceDefinitionOption {
	return func(r *Definition) {
		r.categories = categories
	}
}

func WithCredentials(creds ...Credential) resourceDefinitionOption {
	return func(r *Definition) {
		r.credentials = creds
	}
}

func (r *Definition) RegisterOperation(name string, fn OperationFunc, opts ...operationOption) *Definition {
	op := NewOperation(name, fn, opts...)
	r.operations[name] = op
	return r
}

// JSON returns the JSON representation of the ResourceDefinition
func (r *Definition) JSON() ([]byte, error) {
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
func (r *Definition) GetOperation(name string) (*Operation, bool) {
	op, ok := r.Operations()[name]
	return op, ok
}

// PropertiesSchema returns the properties schema for the resource
func (r *Definition) PropertiesSchema() *jsonschema.Schema {
	return r.properties
}

// LifecycleStage returns the lifecycle stage of the resource
func (r *Definition) LifecycleStage() LifecycleStage {
	return r.lifecycleStage
}

// Categories returns the categories of the resource
func (r *Definition) Categories() []Category {
	return r.categories
}

// Links returns the links associated with the resource
func (r *Definition) Links() []Link {
	return r.links
}

// Credentials returns the credentials associated with the resource
func (r *Definition) Credentials() []Credential {
	return r.credentials
}

// InstructionsMarkdown returns the instructions for the resource
func (r *Definition) InstructionsMarkdown() string {
	return r.instructionsMarkdown
}

// HasHealthCheck returns true if the resource has a health check
func (r *Definition) HasHealthCheck() bool {
	return r.healthCheck != nil
}
