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
			"description": "The unique identifier of the GitLab repository.",
			"type": "string"
		},
		"name": {
			"title": "Repository Name",
			"description": "The name of the GitLab repository.",
			"type": "string"
		},
		"full_name": {
			"title": "Full Repository Name",
			"description": "The full name of the GitLab repository (namespace/project).",
			"type": "string"
		},
		"url": {
			"title": "Repository URL",
			"description": "The HTML URL of the GitLab repository.",
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
			"description": "The description of the GitLab repository.",
			"type": "string"
		},
		"default_branch": {
			"title": "Default Branch",
			"description": "The default branch of the GitLab repository.",
			"type": "string"
		},
		"provider": {
			"title": "Provider",
			"description": "The provider of the repository (always 'gitlab').",
			"type": "string",
			"enum": ["gitlab"]
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
		"namespace": {
			"title": "Namespace",
			"description": "The GitLab namespace (user or group) that will own the repository. If not provided, the repository will be created under the authenticated user.",
			"type": "string"
		},
		"visibility": {
			"title": "Visibility",
			"description": "Whether the repository is private, internal, or public.",
			"type": "string", 
			"enum": ["private", "internal", "public"],
			"default": "private"
		},
		"initialize_with_readme": {
			"title": "Initialize with README",
			"description": "Whether to initialize the repository with a README file.",
			"type": "boolean",
			"default": true
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
			"description": "The unique identifier of the GitLab project (namespace/project or project ID).",
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
			"description": "The unique identifier of the GitLab project (namespace/project or project ID).",
			"type": "string"
		}
	},
	"required": ["externalId"],
	"additionalProperties": false
}`

	repositoryInstructionsMarkdown = `
  _______  ___   _______  ___      _______  ______   
 |       ||   | |       ||   |    |   _   ||    _ |  
 |    ___||   | |_     _||   |    |  |_|  ||   | ||  
 |   | __ |   |   |   |  |   |    |       ||   |_||_ 
 |   ||  ||   |   |   |  |   |___ |       ||    __  |
 |   |_| ||   |   |   |  |       ||   _   ||   |__| |
 |_______||___|   |___|  |_______||__| |__||________|

# GitLab Repository

This resource represents a GitLab project (repository).
`
)
