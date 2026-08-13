package event

import "github.com/content-services/content-sources-backend/pkg/api"

const (
	NotificationVersion     = "v1.2.0"
	NotificationBundle      = "rhel"
	NotificationApplication = "content-sources"
)

type NotificationAction struct {
	Version     string                  `json:"version"`
	Bundle      string                  `json:"bundle"`
	Application string                  `json:"application"`
	EventType   string                  `json:"event_type"`
	Timestamp   string                  `json:"timestamp"`
	OrgID       string                  `json:"org_id"`
	Severity    string                  `json:"severity,omitempty"`
	Context     map[string]any          `json:"context"`
	Events      []NotificationEvent     `json:"events"`
	Recipients  []NotificationRecipient `json:"recipients,omitempty"`
}

type NotificationRecipient struct {
	OnlyAdmins            bool     `json:"only_admins"`
	IgnoreUserPreferences bool     `json:"ignore_user_preferences"`
	Users                 []string `json:"users,omitempty"`
}

type NotificationEvent struct {
	Metadata map[string]any `json:"metadata"`
	Payload  any            `json:"payload"`
}

type RepositoryPayload struct {
	UUID                         string   `json:"uuid"`
	Name                         string   `json:"name"`
	URL                          string   `json:"url"`
	Status                       string   `json:"status,omitempty"`
	DistributionVersions         []string `json:"distribution_versions,omitempty"`
	DistributionArch             string   `json:"distribution_arch,omitempty"`
	GpgKey                       string   `json:"gpg_key,omitempty"`
	MetadataVerification         bool     `json:"metadata_verification"`
	PackageCount                 int      `json:"package_count"`
	FailedIntrospectionsCount    int      `json:"failed_introspections_count"`
	LastIntrospectionError       string   `json:"last_introspection_error,omitempty"`
	LastIntrospectionTime        string   `json:"last_introspection_time,omitempty"`
	LastSuccessIntrospectionTime string   `json:"last_success_introspection_time,omitempty"`
	LastUpdateIntrospectionTime  string   `json:"last_update_introspection_time,omitempty"`
}

func MapRepositoryPayload(r api.RepositoryResponse) RepositoryPayload {
	return RepositoryPayload{
		UUID:                         r.UUID,
		Name:                         r.Name,
		URL:                          r.URL,
		Status:                       r.Status,
		DistributionVersions:         r.DistributionVersions,
		DistributionArch:             r.DistributionArch,
		GpgKey:                       r.GpgKey,
		MetadataVerification:         r.MetadataVerification,
		PackageCount:                 r.PackageCount,
		FailedIntrospectionsCount:    r.FailedIntrospectionsCount,
		LastIntrospectionError:       r.LastIntrospectionError,
		LastIntrospectionTime:        r.LastIntrospectionTime,
		LastSuccessIntrospectionTime: r.LastIntrospectionSuccessTime,
		LastUpdateIntrospectionTime:  r.LastIntrospectionUpdateTime,
	}
}
