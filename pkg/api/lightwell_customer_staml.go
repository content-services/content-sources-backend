package api

import "time"

// LightwellCustomerStamlRequest is the payload to add or remove a STAML to CID mapping.
type LightwellCustomerStamlRequest struct {
	CustomerID string `json:"customer_id"` // Lightwell customer ID (CID)
	Staml      string `json:"staml"`       // STAML identifier
}

// LightwellCustomerStamlResponse is a stored STAML to CID mapping.
type LightwellCustomerStamlResponse struct {
	CustomerID string    `json:"customer_id"` // Lightwell customer ID (CID)
	Staml      string    `json:"staml"`       // STAML identifier
	CreatedAt  time.Time `json:"created_at"`  // When the mapping was created
}
