package googlecloud

const (
	serviceAccountCredentialsSchema = `
{
	"type": "object",
	"properties": {
		"serviceAccountCreds": {
			"title": "Service Account Credentials",
			"description": "Just SA key",
			"type": "string"
		}
	},
	"required": ["serviceAccountCreds"],
	"additionalProperties": false
}
`
)
