package googlecloud

// Schemas for the bucket resource
const (
	bucketPropertiesSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-properties-schema.json",
	"type": "object",
	"properties": {
		"id": {
			"title": "Google Cloud Identifier",
			"description": "The unique identifier for the resource within Google Cloud Platform.",
			"type": "string"
		},
		"name": {
			"title": "Bucket Name",
			"description": "The name of the bucket.",
			"type": "string"
		},
		"location": {
			"title": "Location",
			"description": "The location of the bucket.",
			"type": "string"
		},
		"storageClass": {
			"title": "Storage Class",
			"description": "The storage class of the bucket.",
			"type": "string"
		}
	},
	"additionalProperties": true
}`

	createBucketSchema = `
{
	"$schema": "https://developer.tempestdx.com/schema/v1/tempest-app-schema.json",
	"type": "object",
	"properties": {
		"name": {
			"title": "Name",
			"description": "The name of the bucket.",
			"type": "string"
		},
		"location": {
			"title": "Location",
			"description": "The location of the bucket.",
			"type": "string",
			"default": "US"
		},
		"storageClass": {
			"title": "Storage Class",
			"description": "The storage class of the bucket.",
			"type": "string",
			"default": "STANDARD"
		}
	},
	"required": ["name"],
	"additionalProperties": false
}`

	bucketInstructionsMarkdown = `
# Google Cloud Storage Bucket

This resource allows you to create and manage Google Cloud Storage buckets.
`
)
