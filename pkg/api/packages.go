package api

// PackageResponse represents the paginated response for packages endpoint
type PackageResponse struct {
	Results []PackageItem `json:"results"`
	Total   int           `json:"total"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

// PackageItem represents a Maven package grouped by group_id and artifact_id
type PackageItem struct {
	Group          string        `json:"group"`
	Name           string        `json:"name"`
	Versions       []string      `json:"versions"`
	LatestReleases []ReleaseInfo `json:"latest_releases"`
}

// ReleaseInfo represents the latest release information for a package version
type ReleaseInfo struct {
	Version   string `json:"version"`
	Release   string `json:"release"`
	CreatedAt string `json:"created_at"`
}
