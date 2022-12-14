package message

// EventHeaderKey represents the header key for the kafka messages.
type EventHeaderKey string

const (
	// HdrType is the type of event that match finally with the schema.
	// In a future refactor this will be removed and simplified
	// making a matching of 1 topic - 1 schema.
	HdrType           EventHeaderKey = "Type"
	HdrTypeIntrospect string         = "Introspect"

	// HdrXRhIdentity is the identity header; this will allow to
	// communicate with other services, and add validations based
	// on the identity header.
	HdrXRhIdentity EventHeaderKey = "X-Rh-Identity"
	// HdrXRhInsightsRequestId is the request id; it is expected this
	// header is propagated so it will allow in the future a distributed
	// tracing for the sync and async api handlers.
	HdrXRhInsightsRequestId EventHeaderKey = "X-Rh-Insights-Request-Id"
)
