package payloads

const Introspect = "introspect"

type IntrospectPayload struct {
	Url   string
	Force bool
}
