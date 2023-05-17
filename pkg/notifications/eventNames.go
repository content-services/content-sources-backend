package notifications

type EventName string

// Add more event names here and to below function as needed
const (
	RepositoryCreated              EventName = "repository-created"
	RepositoryIntrospected         EventName = "repository-introspected"
	RepositoryUpdated              EventName = "repository-updated"
	RepositoryIntrospectionFailure EventName = "repository-introspection-failure"
	RepositoryDeleted              EventName = "repository-deleted"
)

func (d EventName) String() string {
	switch d {
	case RepositoryCreated:
		return "repository-created"
	case RepositoryIntrospected:
		return "repository-introspected"
	case RepositoryUpdated:
		return "repository-updated"
	case RepositoryIntrospectionFailure:
		return "repository-introspection-failure"
	case RepositoryDeleted:
		return "repository-deleted"
	// Add more cases here when expanding EventName Enum above
	default:
		return ""
	}
}
