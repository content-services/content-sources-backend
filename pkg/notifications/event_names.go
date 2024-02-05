package notifications

type EventName int

// Add more event names here and to below function as needed
const (
	RepositoryCreated EventName = iota
	RepositoryIntrospected
	RepositoryUpdated
	RepositoryIntrospectionFailure
	RepositoryDeleted
	TemplateCreated
	TemplateUpdated
	TemplateDeleted
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
	case TemplateCreated:
		return "template-created"
	case TemplateUpdated:
		return "template-updated"
	case TemplateDeleted:
		return "template-deleted"
	// Add more cases here when expanding EventName enum above
	default:
		return ""
	}
}
