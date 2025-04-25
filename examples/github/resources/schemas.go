package resources

// Schemas for the repository resource
const (
	repositoryPropertiesSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-properties-schema.json",
	"type": "object",
	"properties": {
		"name": {
			"title": "Repository Name",
			"description": "The name of the GitHub repository.",
			"type": "string"
		},
		"url": {
			"title": "URL",
			"description": "The URL of the GitHub repository.",
			"type": "string"
		}
	},
	"additionalProperties": true
}`

	createRepositorySchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"name": {
			"title": "Name",
			"description": "The desired name of the repository.",
			"type": "string"
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`

	repositoryInstructionsMarkdown = `
# GitHub Repository

This resource represents a GitHub repository (hardcoded example).
`
)
