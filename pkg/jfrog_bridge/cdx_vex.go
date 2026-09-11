package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type cdxBOM struct {
	BOMFormat       string             `json:"bomFormat"`
	SpecVersion     string             `json:"specVersion"`
	Version         int                `json:"version"`
	SerialNumber    string             `json:"serialNumber"`
	Metadata        cdxMetadata        `json:"metadata"`
	Components      []cdxComponent     `json:"components,omitempty"`
	Vulnerabilities []cdxVulnerability `json:"vulnerabilities"`
}

type cdxMetadata struct {
	Timestamp string       `json:"timestamp"`
	Tools     cdxTools     `json:"tools"`
	Supplier  cdxSupplier  `json:"supplier"`
	Component cdxComponent `json:"component"`
}

type cdxTools struct {
	Components []cdxToolComponent `json:"components"`
}

type cdxToolComponent struct {
	Type     string      `json:"type"`
	Name     string      `json:"name"`
	Version  string      `json:"version"`
	Supplier cdxSupplier `json:"supplier"`
}

type cdxSupplier struct {
	Name string   `json:"name"`
	URL  []string `json:"url,omitempty"`
}

type cdxComponent struct {
	Type       string        `json:"type"`
	BOMRef     string        `json:"bom-ref"`
	Group      string        `json:"group"`
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	PackageURL string        `json:"purl"`
	Properties []cdxProperty `json:"properties,omitempty"`
	Pedigree   *cdxPedigree  `json:"pedigree,omitempty"`
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
	BOMRef      string       `json:"bom-ref"`
	ID          string       `json:"id"`
	Source      cdxSource    `json:"source"`
	Ratings     []cdxRating  `json:"ratings"`
	Description string       `json:"description"`
	Analysis    cdxAnalysis  `json:"analysis"`
	Affects     []cdxAffects `json:"affects"`
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
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type cdxAffects struct {
	Ref string `json:"ref"`
}

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
		PackageURL: fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar", group, artifact, baseVersion),
	}

	patches := make([]cdxPatch, 0, len(records))
	for _, rec := range records {
		issue := cdxIssue{
			Type:        "security",
			ID:          rec.CVEID,
			Description: rec.Description,
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
		Notes:     fmt.Sprintf("Backported security fixes to %s %s. Build %s.", artifact, baseVersion, version),
	}
}

// GenerateCycloneDXVEX produces a CycloneDX 1.6 VEX document.
func GenerateCycloneDXVEX(group, artifact, version, baseVersion string, records []OSVRecord) ([]byte, error) {
	serialUUID, _ := uuid.NewRandom()
	bomRef := fmt.Sprintf("%s-rhlw-%s", artifact, strings.ReplaceAll(version, ".", "-"))

	metaComponent := cdxComponent{
		Type:       "library",
		BOMRef:     bomRef,
		Group:      group,
		Name:       artifact,
		Version:    version,
		PackageURL: fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar", group, artifact, version),
		Properties: []cdxProperty{
			{
				Name:  "compatible-with-1",
				Value: fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, baseVersion),
			},
		},
	}

	doc := cdxBOM{
		BOMFormat:    "CycloneDX",
		SpecVersion:  "1.6",
		Version:      1,
		SerialNumber: "urn:uuid:" + serialUUID.String(),
		Metadata: cdxMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Tools: cdxTools{
				Components: []cdxToolComponent{
					{
						Type:    "application",
						Name:    "lightwell-vex-generator",
						Version: "1.0.0",
						Supplier: cdxSupplier{
							Name: "Red Hat Lightwell",
						},
					},
				},
			},
			Supplier: cdxSupplier{
				Name: "Red Hat",
				URL:  []string{"https://www.redhat.com"},
			},
			Component: metaComponent,
		},
	}

	pedigree := buildPedigree(group, artifact, version, baseVersion, records)
	if pedigree != nil {
		comp := metaComponent
		comp.Pedigree = pedigree
		doc.Components = []cdxComponent{comp}
	}

	vulns := make([]cdxVulnerability, 0, len(records))
	for _, rec := range records {
		source := cdxSource{Name: "NVD", URL: "https://nvd.nist.gov/vuln/detail/" + rec.CVEID}
		method := "CVSSv31"
		if rec.CVSSVector != "" && !strings.HasPrefix(rec.CVSSVector, "CVSS:3") {
			method = "CVSSv2"
		}

		vuln := cdxVulnerability{
			BOMRef: "vuln-" + rec.CVEID,
			ID:     rec.CVEID,
			Source: source,
			Ratings: []cdxRating{
				{
					Source:   source,
					Score:    rec.CVSSScore,
					Severity: rec.Severity,
					Method:   method,
					Vector:   rec.CVSSVector,
				},
			},
			Description: rec.Description,
			Analysis: cdxAnalysis{
				State:  "resolved",
				Detail: fmt.Sprintf("Backport of upstream fix applied by Red Hat Lightwell in %s %s", artifact, version),
			},
			Affects: []cdxAffects{
				{Ref: bomRef},
			},
		}
		vulns = append(vulns, vuln)
	}
	doc.Vulnerabilities = vulns

	return json.MarshalIndent(doc, "", "  ")
}
