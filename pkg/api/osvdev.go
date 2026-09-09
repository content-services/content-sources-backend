package api

// OSV (Open Source Vulnerabilities) API/schema types.
//
// These mirror the osv.dev REST API (https://google.github.io/osv.dev/api/) and the
// OSV schema (https://ossf.github.io/osv-schema/) so that OSV tooling such as
// osv-scanner can consume our Lightwell advisories by pointing at /demo/osvdev
// instead of api.osv.dev. Field names use snake_case to match the real service.

// OsvPackage identifies an affected package.
type OsvPackage struct {
	Ecosystem string `json:"ecosystem,omitempty"`
	Name      string `json:"name,omitempty"`
	Purl      string `json:"purl,omitempty"`
}

// OsvEvent is a single point in an affected range. Exactly one field is set.
type OsvEvent struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
	Limit        string `json:"limit,omitempty"`
}

// OsvRange is an ordered set of version events for an ecosystem.
type OsvRange struct {
	Type   string     `json:"type"`
	Repo   string     `json:"repo,omitempty"`
	Events []OsvEvent `json:"events"`
}

// OsvAffected describes a package and the versions it is affected in.
type OsvAffected struct {
	Package  OsvPackage `json:"package"`
	Ranges   []OsvRange `json:"ranges,omitempty"`
	Versions []string   `json:"versions,omitempty"`
}

// OsvReference is a URL reference for a vulnerability.
type OsvReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// OsvSeverity is a severity score for a vulnerability.
type OsvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// OsvVulnerability is a full OSV record.
type OsvVulnerability struct {
	SchemaVersion string         `json:"schema_version,omitempty"`
	ID            string         `json:"id"`
	Modified      string         `json:"modified"`
	Published     string         `json:"published,omitempty"`
	Aliases       []string       `json:"aliases,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Details       string         `json:"details,omitempty"`
	Severity      []OsvSeverity  `json:"severity,omitempty"`
	Affected      []OsvAffected  `json:"affected,omitempty"`
	References    []OsvReference `json:"references,omitempty"`
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

// OsvVulnerabilityList is the response of POST /v1/query.
type OsvVulnerabilityList struct {
	Vulns         []OsvVulnerability `json:"vulns,omitempty"`
	NextPageToken string             `json:"next_page_token,omitempty"`
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
