package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CycloneDX 1.6 VEX document structure
type cdxVEX struct {
	BOMFormat    string             `json:"bomFormat"`
	SpecVersion  string             `json:"specVersion"`
	Version      int                `json:"version"`
	SerialNumber string             `json:"serialNumber"`
	Metadata     cdxMetadata        `json:"metadata"`
	Vulns        []cdxVulnerability `json:"vulnerabilities"`
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
	PURL       string        `json:"purl"`
	Properties []cdxProperty `json:"properties"`
}

type cdxProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cdxVulnerability struct {
	BOMRef      string        `json:"bom-ref"`
	ID          string        `json:"id"`
	Source       cdxSource     `json:"source"`
	Ratings     []cdxRating   `json:"ratings"`
	Description string        `json:"description"`
	Analysis    cdxAnalysis   `json:"analysis"`
	Affects     []cdxAffects  `json:"affects"`
}

type cdxSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type cdxRating struct {
	Source   cdxSource `json:"source"`
	Score    float64   `json:"score"`
	Severity string   `json:"severity"`
	Method   string   `json:"method"`
	Vector   string   `json:"vector"`
}

type cdxAnalysis struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type cdxAffects struct {
	Ref string `json:"ref"`
}

// GenerateCycloneDXVEX produces a CycloneDX 1.6 VEX document.
func GenerateCycloneDXVEX(group, artifact, version, baseVersion string, records []OSVRecord) ([]byte, error) {
	serialUUID, _ := uuid.NewRandom()
	bomRef := fmt.Sprintf("%s-rhlw-%s", artifact, strings.ReplaceAll(version, ".", "-"))

	doc := cdxVEX{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.6",
		Version:     1,
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
			Component: cdxComponent{
				Type:    "library",
				BOMRef:  bomRef,
				Group:   group,
				Name:    artifact,
				Version: version,
				PURL:    fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar", group, artifact, version),
				Properties: []cdxProperty{
					{
						Name:  "compatible-with-1",
						Value: fmt.Sprintf("pkg:maven/%s/%s@%s", group, artifact, baseVersion),
					},
				},
			},
		},
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
			Source:  source,
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
	doc.Vulns = vulns

	return json.MarshalIndent(doc, "", "  ")
}
