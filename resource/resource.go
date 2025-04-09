package resource

// ResourceRef is a reference to a resource.
type ResourceRef struct {
	ExternalID  string         `json:"externalId"`
	Name        string         `json:"name"`
	// TODO: consider Links here
}
// Resource represents a generic resource with properties and links.
type Resource struct {
	ResourceRef
	DisplayName string         `json:"displayName"`
	Properties  map[string]any `json:"properties"`
	Category    Category       `json:"category"`
	Links       []*Link        `json:"links"`
}

// ResourceCollection represents a collection of resources with pagination
type ResourceCollection struct {
	Items      []*Resource `json:"items"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination holds metadata for paginated responses.
type Pagination struct {
	TotalCount int    `json:"totalCount"`           // Total number of items available in the dataset
	PageSize   int    `json:"pageSize"`             // Number of items per page
	HasMore    bool   `json:"hasMore"`              // Indicates if there are more items available
	NextCursor string `json:"nextCursor,omitempty"` // Opaque cursor for the next page
}

// ResponseData is an interface to group *Resource and *ResourceCollection
// This is used in the OperationResponse struct to allow for a single type for the data field.
type ResponseData interface {
	IsResponseData()
}

func (*Resource) IsResponseData()           {}
func (*ResourceCollection) IsResponseData() {}
