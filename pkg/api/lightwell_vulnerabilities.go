package api

import "time"

// LightwellVulnerabilityResponse is a single vulnerability in list responses.
type LightwellVulnerabilityResponse struct {
	UUID               string    `json:"uuid"`                        // UUID of the vulnerability
	VulnerabilityID    string    `json:"vulnerability_id"`            // Business identifier (e.g. LWL-2026-4401)
	Purl               *string   `json:"purl,omitempty"`              // Package URL
	ComponentName      string    `json:"component_name"`              // Component / package name
	Package            string    `json:"package"`                     // Alias of component_name
	ComponentVersion   string    `json:"component_version"`           // Component version
	Title              *string   `json:"title,omitempty"`             // Vulnerability title
	Cwe                *string   `json:"cwe,omitempty"`               // CWE identifier
	Description        *string   `json:"description,omitempty"`       // Vulnerability description
	Severity           string    `json:"severity"`                    // Severity (Critical, Important, Moderate, Low)
	Cvss               *float64  `json:"cvss,omitempty"`              // CVSS score
	CvssVector         *string   `json:"cvss_vector,omitempty"`       // CVSS vector string
	ExploitTested      bool      `json:"exploit_tested"`              // Whether an exploit was tested
	ReproducerIncluded bool      `json:"reproducer_included"`         // Whether a reproducer is included
	CustomerPriority   *string   `json:"customer_priority,omitempty"` // Customer priority
	Status             string    `json:"status"`                      // Workflow status
	Language           *string   `json:"language,omitempty"`          // Derived language (java, python, javascript, csharp)
	Complexity         string    `json:"complexity"`                  // Standard, Complex, or Extensive
	SubmittedDate      time.Time `json:"submitted_date"`              // Date the vulnerability was submitted
	LastUpdated        time.Time `json:"last_updated"`                // Last update timestamp
	AgeDays            int       `json:"age_days"`                    // UTC calendar days since submitted_date
	Embargo            bool      `json:"embargo"`                     // Embargo flag
	Duplicate          bool      `json:"duplicate"`                   // Duplicate flag
	DuplicateOf        *string   `json:"duplicate_of,omitempty"`      // Canonical vulnerability_id when duplicate
	LtwlsuptTicketIDs  []string  `json:"ltwlsupt_ticket_ids"`         // Lightwell support ticket IDs
}

// LightwellVulnerabilityCollectionMeta extends pagination metadata with list aggregates.
type LightwellVulnerabilityCollectionMeta struct {
	ResponseMetadata
	CriticalCount int64            `json:"critical_count"` // Count of Critical severity rows matching filters
	EmbargoCount  int64            `json:"embargo_count"`  // Count of embargoed rows matching filters
	StatusCounts  map[string]int64 `json:"status_counts"`  // Per-status counts matching filters
}

// LightwellVulnerabilityCollectionResponse is a paginated list of vulnerabilities.
type LightwellVulnerabilityCollectionResponse struct {
	Data  []LightwellVulnerabilityResponse     `json:"data"`  // Requested Data
	Meta  LightwellVulnerabilityCollectionMeta `json:"meta"`  // Metadata about the request
	Links Links                                `json:"links"` // Links to other pages of results
}

func (r *LightwellVulnerabilityCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta.Limit = meta.Limit
	r.Meta.Offset = meta.Offset
	r.Meta.Count = meta.Count
	r.Links = links
}

// LightwellCustomerIdsResponse is the list of customer IDs that have vulnerabilities.
type LightwellCustomerIdsResponse struct {
	Data []string `json:"data"` // Customer IDs
}

// LightwellLtwlsuptTicketIdsResponse is the distinct Lightwell support ticket IDs for a customer.
type LightwellLtwlsuptTicketIdsResponse struct {
	Data []string `json:"data"` // Lightwell support ticket IDs
}
