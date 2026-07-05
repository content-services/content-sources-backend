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

type ListPackagesRequest struct {
	UUID   string `param:"uuid" validate:"required"` // Identifier of the repository
	Search string `query:"search"`                   // Name or group to optionally filter-on
}

// ReleaseInfo represents the latest release information for a package version
type ReleaseInfo struct {
	Version   string `json:"version"`
	Release   string `json:"release"`
	CreatedAt string `json:"created_at"`
}

// MavenPackageDetailResponse represents the detail response for a specific Maven package.
type MavenPackageDetailResponse struct {
	Group      string        `json:"group"`
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	Builds     []ReleaseInfo `json:"builds"`
	Summary    *string       `json:"summary,omitempty"`
	License    *string       `json:"license,omitempty"`
	ProjectURL *string       `json:"project_url,omitempty"`
	Author     *string       `json:"author,omitempty"`
}

// PythonPackageAuthor represents package authorship metadata.
type PythonPackageAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// PythonDistribution represents a distribution file for a Python package version.
type PythonDistribution struct {
	Name          string `json:"name"`
	Filename      string `json:"filename"`
	PackageType   string `json:"packagetype"`
	PythonVersion string `json:"python_version"`
	Sha256        string `json:"sha256"`
	Size          int64  `json:"size"`
	CreatedAt     string `json:"created_at"`
}

// PythonPackageDetailResponse represents the detail response for a specific Python package.
type PythonPackageDetailResponse struct {
	Name             string               `json:"name"`
	Version          string               `json:"version"`
	Summary          string               `json:"summary"`
	Description      string               `json:"description"`
	LastUpdated      string               `json:"last_updated"`
	License          string               `json:"license"`
	Author           PythonPackageAuthor  `json:"author"`
	UpstreamVersions []string             `json:"upstream_versions"`
	ProjectURL       string               `json:"project_url"`
	Distributions    []PythonDistribution `json:"distributions"`
}
