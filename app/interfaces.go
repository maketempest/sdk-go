package app

// InterfaceType represents a predefined plug and play interface name.
type InterfaceType string

const (
	// GitRepositoryInterface represents the interface for Git repositories.
	GitRepositoryInterface InterfaceType = "git_repository"
	// Add other interface types here in the future
)

// validInterfaces maps the valid interface types for validation.
var validInterfaces = map[InterfaceType]bool{
	GitRepositoryInterface: true,
	// Add other valid interfaces here
}

// IsValid checks if the interface type is one of the predefined valid types.
func (it InterfaceType) IsValid() bool {
	_, ok := validInterfaces[it]
	return ok
}
