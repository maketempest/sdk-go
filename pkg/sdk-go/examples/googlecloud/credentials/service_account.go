package googlecloud

import (
	"github.com/tempestdx/sdk-go/credential"
	"github.com/tempestdx/sdk-go/jsonschema"
)

const (
	SaProviderName = "service-account-google-cloud"
)

func NewServiceAccountCredentialProvider() *credential.CredentialProvider {
	return &credential.CredentialProvider{
		Name:   SaProviderName,
		Type:   credential.CredentialTypeAPIKey,
		Schema: jsonschema.MustParseSchema([]byte(serviceAccountCredentialsSchema)),
	}
}
