package resource

import "github.com/tempestdx/sdk-go/jsonschema"

// Credential represents an authentication credential
// TODO: Should be settable on app/resource/operation level
// Right now it's settable only on resource level
// Tempest should automatically add credentials to the operation request from the lowest available level
type Credential struct {
	Name   string             `json:"name"`
	Type   CredentialType     `json:"type"`
	Schema *jsonschema.Schema `json:"schema"`
}

func (c *Credential) isValid() bool {
	if c.Name == "" {
		return false
	}

	if c.Type != CredentialTypeAPIKey {
		return false
	}

	return true
}

func (c *Credential) setDefault() {
	if c.Type == "" {
		c.Type = CredentialTypeAPIKey
	}
}

type CredentialType string

const (
	// CredentialTypeAPIKey represents an API key credential
	CredentialTypeAPIKey CredentialType = "api_key"
)
