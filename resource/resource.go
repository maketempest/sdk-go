package resource

type Resource struct {
	ExternalID  string
	Name        string
	DisplayName string
	Properties  map[string]any
	Category    Category
	Links       []*Link
}
