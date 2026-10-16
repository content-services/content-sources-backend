package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// cdxBOM is the top-level CycloneDX document for both SBOM and VEX.
type cdxBOM struct {
	BOMFormat       string             `json:"bomFormat"`
	SpecVersion     string             `json:"specVersion"`
	Version         int                `json:"version"`
	SerialNumber    string             `json:"serialNumber"`
	Metadata        *cdxMetadata       `json:"metadata,omitempty"`
	Components      []cdxComponent     `json:"components,omitempty"`
	Vulnerabilities []cdxVulnerability `json:"vulnerabilities,omitempty"`
}

type cdxMetadata struct {
	Timestamp string `json:"timestamp"`
}

type cdxComponent struct {
	Type       string        `json:"type"`
	BOMRef     string        `json:"bom-ref"`
	Group      string        `json:"group"`
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	PackageURL string        `json:"purl"`
	Hashes     []cdxHash     `json:"hashes,omitempty"`
	Licenses   []cdxLicense  `json:"licenses,omitempty"`
	Properties []cdxProperty `json:"properties,omitempty"`
	Pedigree   *cdxPedigree  `json:"pedigree,omitempty"`
}

type cdxHash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
}

type cdxLicense struct {
	License cdxLicenseID `json:"license"`
}

type cdxLicenseID struct {
	ID string `json:"id"`
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cdxPedigree struct {
	Ancestors []cdxComponent `json:"ancestors,omitempty"`
	Patches   []cdxPatch     `json:"patches,omitempty"`
	Notes     string         `json:"notes,omitempty"`
}

type cdxPatch struct {
	Type     string     `json:"type"`
	Resolves []cdxIssue `json:"resolves,omitempty"`
}

type cdxIssue struct {
	Type        string    `json:"type"`
	ID          string    `json:"id"`
	Description string    `json:"description,omitempty"`
	Source      cdxSource `json:"source,omitempty"`
}

type cdxVulnerability struct {
	BOMRef         string         `json:"bom-ref"`
	ID             string         `json:"id"`
	Source         cdxSource      `json:"source"`
	Ratings        []cdxRating    `json:"ratings"`
	Description    string         `json:"description,omitempty"`
	Analysis       cdxAnalysis    `json:"analysis"`
	Affects        []cdxAffects   `json:"affects"`
	Response       []string       `json:"response,omitempty"`
	References     []cdxReference `json:"references,omitempty"`
	Advisories     []cdxAdvisory  `json:"advisories,omitempty"`
	Recommendation string         `json:"recommendation,omitempty"`
	Published      string         `json:"published,omitempty"`
	Updated        string         `json:"updated,omitempty"`
	// LWLP-2370: CWE field built but output disabled. Enable when CWE + CSAF
	// enrichment are confirmed ready together.
	CWEs []int `json:"-"`
}

type cdxSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type cdxRating struct {
	Source   cdxSource `json:"source"`
	Score    float64   `json:"score"`
	Severity string    `json:"severity"`
	Method   string    `json:"method"`
	Vector   string    `json:"vector"`
}

type cdxAnalysis struct {
	State    string   `json:"state"`
	Response []string `json:"response,omitempty"`
	Detail   string   `json:"detail"`
}

type cdxAffects struct {
	Ref      string       `json:"ref"`
	Versions []cdxVersion `json:"versions,omitempty"`
}

type cdxVersion struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

type cdxReference struct {
	ID     string    `json:"id"`
	Source cdxSource `json:"source"`
}

type cdxAdvisory struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

const repositoryURL = "https://packages.redhat.com/lightwell/java/remediated/"

// buildPedigree constructs a CycloneDX pedigree with ancestors and patches.
// Returns nil when version has no .rhlw-* suffix (baseVersion == version).
func buildPedigree(group, artifact, version, baseVersion string, records []OSVRecord) *cdxPedigree {
	if baseVersion == version {
		return nil
	}

	ancestor := cdxComponent{
		Type:       "library",
		Group:      group,
		Name:       artifact,
		Version:    baseVersion,
		PackageURL: fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, baseVersion),
	}

	patches := make([]cdxPatch, 0, len(records))
	for _, rec := range records {
		issue := cdxIssue{
			Type: "security",
			ID:   rec.CVEID,
			Source: cdxSource{
				Name: "NVD",
				URL:  "https://nvd.nist.gov/vuln/detail/" + rec.CVEID,
			},
		}
		patches = append(patches, cdxPatch{
			Type:     "backport",
			Resolves: []cdxIssue{issue},
		})
	}

	return &cdxPedigree{
		Ancestors: []cdxComponent{ancestor},
		Patches:   patches,
		Notes:     fmt.Sprintf("Lightwell remediation: backported security fixes to %s %s. Build %s.", artifact, baseVersion, version),
	}
}

// buildAffectsRef constructs the URN cross-reference to an SBOM.
// Format: urn:cdx:<sbom-uuid>/<version>#<purl-encoded>
func buildAffectsRef(sbomSerialNumber string, bomVersion int, purl string) string {
	sbomUUID := strings.TrimPrefix(sbomSerialNumber, "urn:uuid:")
	return fmt.Sprintf("urn:cdx:%s/%d#%s", sbomUUID, bomVersion, url.QueryEscape(purl))
}

// buildAdvisories returns NVD + upstream advisory links from OSV data.
func buildAdvisories(rec OSVRecord) []cdxAdvisory {
	advisories := []cdxAdvisory{
		{
			URL:   "https://nvd.nist.gov/vuln/detail/" + rec.CVEID,
			Title: "NVD entry for " + rec.CVEID,
		},
	}
	if rec.IssueURL != "" {
		advisories = append(advisories, cdxAdvisory{
			URL:   rec.IssueURL,
			Title: "Upstream issue report",
		})
	}
	if rec.FixCommitURL != "" {
		advisories = append(advisories, cdxAdvisory{
			URL:   rec.FixCommitURL,
			Title: "Upstream fix commit",
		})
	}
	return advisories
}

// GenerateCycloneDXSBOM produces a per-artifact CycloneDX 1.7 SBOM.
// Returns the serialNumber and the serialized JSON.
func GenerateCycloneDXSBOM(group, artifact, version, baseVersion, sha256hex string, records []OSVRecord) (string, []byte, error) {
	serialUUID, _ := uuid.NewRandom()
	serialNumber := "urn:uuid:" + serialUUID.String()

	purl := fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar&repository_url=%s",
		group, artifact, version, repositoryURL)
	bomRef := fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, version)

	comp := cdxComponent{
		Type:       "library",
		BOMRef:     bomRef,
		Group:      group,
		Name:       artifact,
		Version:    version,
		PackageURL: purl,
		Licenses:   []cdxLicense{},
	}

	if sha256hex != "" {
		comp.Hashes = []cdxHash{{Alg: "SHA-256", Content: sha256hex}}
	}

	pedigree := buildPedigree(group, artifact, version, baseVersion, records)
	if pedigree != nil {
		comp.Pedigree = pedigree
	}

	doc := cdxBOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  "1.7",
		Version:      1,
		SerialNumber: serialNumber,
		Metadata:     &cdxMetadata{Timestamp: time.Now().UTC().Format(time.RFC3339)},
		Components:   []cdxComponent{comp},
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	return serialNumber, data, err
}

// GenerateCycloneDXVEX produces a CycloneDX 1.7 VEX document.
// sbomSerialNumber is used for the affects[].ref cross-reference to the SBOM.
func GenerateCycloneDXVEX(group, artifact, version, baseVersion, sbomSerialNumber string, records []OSVRecord) ([]byte, error) {
	serialUUID, _ := uuid.NewRandom()

	purl := fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar&repository_url=%s",
		group, artifact, version, repositoryURL)
	timestamp := time.Now().UTC().Format(time.RFC3339)

	vulns := make([]cdxVulnerability, 0, len(records))
	for _, rec := range records {
		source := cdxSource{
			Name: "NVD",
			URL:  "https://nvd.nist.gov/vuln/detail/" + rec.CVEID,
		}
		method := "CVSSv31"
		if rec.CVSSVector != "" && !strings.HasPrefix(rec.CVSSVector, "CVSS:3") {
			method = "CVSSv2"
		}

		recommendation := fmt.Sprintf(
			"Update to %s %s from Red Hat Lightwell remediated repository. "+
				"This version backports the upstream fix from %s %s.",
			artifact, version, artifact, baseVersion,
		)

		affectsRef := ""
		if sbomSerialNumber != "" {
			affectsRef = buildAffectsRef(sbomSerialNumber, 1, purl)
		} else {
			affectsRef = fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, version)
		}

		vulnUUID, _ := uuid.NewRandom()
		vuln := cdxVulnerability{
			BOMRef: vulnUUID.String(),
			ID:     rec.CVEID,
			Source: source,
			Ratings: []cdxRating{
				{
					Source:   cdxSource{Name: "Red Hat Lightwell"},
					Score:    rec.CVSSScore,
					Severity: rec.Severity,
					Method:   method,
					Vector:   rec.CVSSVector,
				},
			},
			Analysis: cdxAnalysis{
				State:    "resolved",
				Response: []string{"update"},
				Detail:   recommendation,
			},
			Affects: []cdxAffects{
				{
					Ref: affectsRef,
					Versions: []cdxVersion{
						{Version: version, Status: "unaffected"},
					},
				},
			},
			References: []cdxReference{
				{
					ID: rec.CVEID,
					Source: cdxSource{
						Name: "NVD entry for " + rec.CVEID,
						URL:  "https://nvd.nist.gov/vuln/detail/" + rec.CVEID,
					},
				},
			},
			Advisories:     buildAdvisories(rec),
			Recommendation: recommendation,
			Published:      timestamp,
			Updated:        timestamp,
		}
		vulns = append(vulns, vuln)
	}

	doc := cdxBOM{
		BOMFormat:       "CycloneDX",
		SpecVersion:     "1.7",
		Version:         1,
		SerialNumber:    "urn:uuid:" + serialUUID.String(),
		Vulnerabilities: vulns,
	}

	return json.MarshalIndent(doc, "", "  ")
}
