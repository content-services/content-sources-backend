package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"time"
)

// OpenVEX v0.2.0 document structure — used as Evidence payload.
type openVEXDocument struct {
	Context   string             `json:"@context"`
	ID        string             `json:"@id"`
	Author    string             `json:"author"`
	Role      string             `json:"role"`
	Timestamp string             `json:"timestamp"`
	Version   int                `json:"version"`
	Tooling   string             `json:"tooling"`
	Stmts     []openVEXStatement `json:"statements"`
}

type openVEXStatement struct {
	Vulnerability openVEXVuln   `json:"vulnerability"`
	Products      []openVEXProd `json:"products"`
	Status        string        `json:"status"`
}

type openVEXVuln struct {
	ID          string   `json:"@id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases,omitempty"`
}

type openVEXProd struct {
	ID          string             `json:"@id"`
	Identifiers openVEXIdentifiers `json:"identifiers"`
}

type openVEXIdentifiers struct {
	PURL string `json:"purl"`
}

// GenerateOpenVEXPredicate produces an OpenVEX v0.2.0 document used
// as the Evidence predicate payload (not uploaded as a file).
func GenerateOpenVEXPredicate(group, artifact, version string, records []OSVRecord) ([]byte, error) {
	purl := fmt.Sprintf("pkg:maven/%s/%s@%s?type=jar", group, artifact, version)

	doc := openVEXDocument{
		Context:   "https://openvex.dev/ns/v0.2.0",
		ID:        fmt.Sprintf("https://packages.redhat.com/lightwell/vex/%s-%s", artifact, version),
		Author:    "Red Hat Lightwell <lightwell@redhat.com>",
		Role:      "fix_vendor",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   1,
		Tooling:   "lightwell-vex-generator/1.0.0",
	}

	stmts := make([]openVEXStatement, 0, len(records))
	for _, rec := range records {
		vulnID := "https://nvd.nist.gov/vuln/detail/" + rec.CVEID

		stmt := openVEXStatement{
			Vulnerability: openVEXVuln{
				ID:          vulnID,
				Name:        rec.CVEID,
				Description: rec.Description,
			},
			Products: []openVEXProd{
				{
					ID:          purl,
					Identifiers: openVEXIdentifiers{PURL: purl},
				},
			},
			Status: "fixed",
		}
		if len(rec.Aliases) > 0 {
			stmt.Vulnerability.Aliases = rec.Aliases
		}
		stmts = append(stmts, stmt)
	}
	doc.Stmts = stmts

	return json.MarshalIndent(doc, "", "  ")
}
