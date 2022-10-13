package message

type EventHeaderKey string

const (
	HdrType           EventHeaderKey = "Type"
	HdrTypeIntrospect string         = "Introspect"

	HdrXRhIdentity          EventHeaderKey = "X-Rh-Identity"
	HdrXRhInsightsRequestId EventHeaderKey = "X-Rh-Insights-Request-Id"
)
