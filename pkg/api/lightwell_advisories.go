package api

type LightwellAdvisoryResponse struct {
	AdvisoryID    string   `json:"advisory_id"`
	Severity      string   `json:"severity"`
	Details       string   `json:"details"`
	ReferenceURLs []string `json:"reference_urls"`
	PackageName   string   `json:"package_name"`
	FixedVersions []string `json:"fixed_versions"`
	Repository    string   `json:"repository"`
}

type LightwellAdvisoryCollectionResponse struct {
	Data  []LightwellAdvisoryResponse `json:"data"`
	Meta  ResponseMetadata            `json:"meta"`
	Links Links                       `json:"links"`
}

func (r *LightwellAdvisoryCollectionResponse) SetMetadata(meta ResponseMetadata, links Links) {
	r.Meta = meta
	r.Links = links
}

type LightwellAdvisoryFilterData struct {
	Repository  string `query:"repository"`
	PackageName string `query:"package_name"`
	SeverityMin string `query:"severity_min"`
	CveID       string `query:"cve_id"`
}
