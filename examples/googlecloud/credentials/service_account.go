package googlecloud

import (
	"github.com/tempestdx/sdk-go/credential"
	"github.com/tempestdx/sdk-go/jsonschema"
)

func NewServiceAccountCredentialProvider() *credential.CredentialProvider {
	return &credential.CredentialProvider{
		Name:   "service-account-google-cloud",
		Type:   credential.CredentialTypeAPIKey,
		Schema: jsonschema.MustParseSchema([]byte(serviceAccountCredentialsSchema)),
	}
}
