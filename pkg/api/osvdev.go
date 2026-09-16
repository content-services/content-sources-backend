package api

import "encoding/json"

// OSV (Open Source Vulnerabilities) API types.
//
// These mirror the osv.dev REST API (https://google.github.io/osv.dev/api/) so
// that OSV tooling such as osv-scanner can consume our Lightwell content by
// pointing at /demo/osvdev instead of api.osv.dev. Field names use snake_case to
// match the real service. The full vulnerability records themselves are served
// verbatim from curated OSV JSON (see pkg/lightwell/osv), so only the request
// types and thin response envelopes are modeled here.

// OsvPackage identifies a package in a query.
type OsvPackage struct {
	Ecosystem string `json:"ecosystem,omitempty"`
	Name      string `json:"name,omitempty"`
	Purl      string `json:"purl,omitempty"`
}

// OsvQuery is the body of POST /v1/query and each entry of a batch query.
type OsvQuery struct {
	Commit    string     `json:"commit,omitempty"`
	Version   string     `json:"version,omitempty"`
	Package   OsvPackage `json:"package,omitempty"`
	PageToken string     `json:"page_token,omitempty"`
}

// OsvBatchQuery is the body of POST /v1/querybatch.
type OsvBatchQuery struct {
	Queries []OsvQuery `json:"queries"`
}

// OsvVulnerabilityList is the response of POST /v1/query. Each vuln is a full OSV
// record served verbatim as raw JSON.
type OsvVulnerabilityList struct {
	Vulns         []json.RawMessage `json:"vulns,omitempty"`
	NextPageToken string            `json:"next_page_token,omitempty"`
}

// OsvVulnStub is the lightweight vulnerability reference returned by querybatch.
type OsvVulnStub struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}

// OsvBatchResult is a single query's result within a batch response.
type OsvBatchResult struct {
	Vulns         []OsvVulnStub `json:"vulns,omitempty"`
	NextPageToken string        `json:"next_page_token,omitempty"`
}

// OsvBatchVulnerabilityList is the response of POST /v1/querybatch, index-aligned
// with the request queries.
type OsvBatchVulnerabilityList struct {
	Results []OsvBatchResult `json:"results"`
}
