package resource

type Metadata struct {
	// TaskID is the ID of the Task that is executing the operation.
	TaskID string `json:"task_id"`
	// ProjectID is the ID of the Tempest Project. This guaranteed to be unique.
	ProjectID string `json:"project_id"`
	// ProjectName is the user defined name of the Tempest Project.
	ProjectName string `json:"project_name"`
	// Owners are the user(s) who created or own the Project.
	Owners []Owner `json:"owners"`
	// Author is the user or team who created the Project.
	Author Owner `json:"author"`
}

type OwnerType string

const (
	OwnerTypeUser OwnerType = "user"
	OwnerTypeTeam OwnerType = "team"
)

type Owner struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Type  OwnerType `json:"type"`
}
