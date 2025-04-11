package credential

import "github.com/tempestdx/sdk-go/jsonschema"

// Credential represents an authentication credential
type CredentialProvider struct {
	Name   string             `json:"name"`
	Type   CredentialType     `json:"type"`
	Schema *jsonschema.Schema `json:"schema"`
}

type CredentialType string

const (
	// CredentialTypeAPIKey represents an API key credential
	CredentialTypeAPIKey CredentialType = "api_key"
)
