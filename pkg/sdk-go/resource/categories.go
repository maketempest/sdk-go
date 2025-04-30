package resource

type Category string

// Predefined categories
const (
	CategoryCompute        Category = "compute"
	CategoryStorage        Category = "storage"
	CategoryNetwork        Category = "network"
	CategorySecurity       Category = "security"
	CategoryDatabase       Category = "database"
	CategoryCodeRepository Category = "code_repository"
)

// validCategories tracks all valid categories, initialized with predefined ones
var validCategories = map[Category]bool{
	CategoryCompute:        true,
	CategoryStorage:        true,
	CategoryNetwork:        true,
	CategorySecurity:       true,
	CategoryDatabase:       true,
	CategoryCodeRepository: true,
}

func isValidCategory(c Category) bool {
	return validCategories[c]
}
