package resources

// Schemas for the repository resource
const (
	repositoryPropertiesSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-properties-schema.json",
	"type": "object",
	"properties": {
		"externalId": {
			"title": "Repository ID",
			"description": "The unique identifier of the GitHub repository.",
			"type": "string"
		},
		"name": {
			"title": "Repository Name",
			"description": "The name of the GitHub repository.",
			"type": "string"
		},
		"full_name": {
			"title": "Full Repository Name",
			"description": "The full name of the GitHub repository (owner/repo).",
			"type": "string"
		},
		"url": {
			"title": "Repository URL",
			"description": "The HTML URL of the GitHub repository.",
			"type": "string",
			"format": "uri"
		},
		"http_clone_url": {
			"title": "HTTP Clone URL",
			"description": "The HTTP URL to clone the repository.",
			"type": "string",
			"format": "uri"
		},
		"ssh_clone_url": {
			"title": "SSH Clone URL",
			"description": "The SSH URL to clone the repository.",
			"type": "string"
		},
		"description": {
			"title": "Description",
			"description": "The description of the GitHub repository.",
			"type": "string"
		},
		"default_branch": {
			"title": "Default Branch",
			"description": "The default branch of the GitHub repository.",
			"type": "string"
		},
		"provider": {
			"title": "Provider",
			"description": "The provider of the repository (always 'github').",
			"type": "string",
			"enum": ["github"]
		}
	},
	"required": ["externalId", "name", "url"],
	"additionalProperties": true
}`

	createRepositorySchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"name": {
			"title": "Repository Name",
			"description": "The desired name of the repository.",
			"type": "string"
		},
		"organization": {
			"title": "Organization",
			"description": "The GitHub organization that will own the repository. If not provided, the repository will be created under the authenticated user.",
			"type": "string"
		},
		"visibility": {
			"title": "Visibility",
			"description": "Whether the repository is private or public.",
			"type": "string", 
			"enum": ["public", "private"],
			"default": "private"
		},
		"template_repository": {
			"title": "Template Repository",
			"description": "The template repository to use when creating this repository (not supported in this version).",
			"type": "string"
		},
		"enable_dependabot": {
			"title": "Enable Dependabot",
			"description": "Whether to enable Dependabot security updates (not supported in this version).",
			"type": "boolean",
			"default": false
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`

	readRepositorySchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"externalId": {
			"title": "Repository ID",
			"description": "The full name of the GitHub repository (owner/repo).",
			"type": "string"
		}
	},
	"required": ["externalId"],
	"additionalProperties": false
}`

	deleteRepositorySchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"externalId": {
			"title": "Repository ID",
			"description": "The full name of the GitHub repository (owner/repo).",
			"type": "string"
		}
	},
	"required": ["externalId"],
	"additionalProperties": false
}`

	repositoryInstructionsMarkdown = `
 _______  ___   _______  __   __  __   __  _______ 
|       ||   | |       ||  | |  ||  | |  ||  _    |
|    ___||   | |_     _||  |_|  ||  | |  || |_|   |
|   | __ |   |   |   |  |       ||  |_|  ||       |
|   ||  ||   |   |   |  |       ||       ||  _   | 
|   |_| ||   |   |   |  |   _   ||       || |_|   |
|_______||___|   |___|  |__| |__||_______||_______|

# GitHub Repository

This resource represents a GitHub repository.
`
)
