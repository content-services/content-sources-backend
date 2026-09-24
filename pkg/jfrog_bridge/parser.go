package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Remediation is the internal representation of a single package release
// extracted from a CloudEvents bridge message.
type Remediation struct {
	GroupID     string
	ArtifactID  string
	Version     string
	BaseVersion string
	CVEsFixed   []string
}

var rhlwSuffix = regexp.MustCompile(`\.rhlw[-.]\d+$`)

func stripRHLWSuffix(version string) string {
	return rhlwSuffix.ReplaceAllString(version, "")
}

// cloudEvent mirrors the CloudEvents v1.0 structure used on the dedicated
// bridge topic (platform.lightwell.advisory-created).
type cloudEvent struct {
	SpecVersion string `json:"specversion"`
	Type        string `json:"type"`
	Data        []struct {
		Payload json.RawMessage `json:"payload"`
	} `json:"data"`
}

type payloadFull struct {
	PackageName string `json:"package_name"`
	Releases    []struct {
		ReleaseNames []struct {
			Name string `json:"name"`
		} `json:"release_names"`
		RelatedCVE []struct {
			CVE      string `json:"cve"`
			Severity string `json:"severity"`
		} `json:"related_cve"`
	} `json:"releases"`
}

// ParseRemediations parses a CloudEvents envelope from the dedicated bridge
// topic (platform.lightwell.advisory-created) into one Remediation per
// (group, artifact, version).
func ParseRemediations(data []byte) ([]Remediation, error) {
	var ce cloudEvent
	if err := json.Unmarshal(data, &ce); err != nil {
		return nil, err
	}
	if ce.SpecVersion == "" || len(ce.Data) == 0 {
		return nil, fmt.Errorf("not a CloudEvents message")
	}

	var result []Remediation
	for _, entry := range ce.Data {
		var p payloadFull
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			continue
		}
		group, artifact, err := splitPackageName(p.PackageName)
		if err != nil {
			continue
		}
		for _, rel := range p.Releases {
			cves := make([]string, 0, len(rel.RelatedCVE))
			for _, c := range rel.RelatedCVE {
				cves = append(cves, c.CVE)
			}
			for _, rn := range rel.ReleaseNames {
				result = append(result, Remediation{
					GroupID:     group,
					ArtifactID:  artifact,
					Version:     rn.Name,
					BaseVersion: stripRHLWSuffix(rn.Name),
					CVEsFixed:   cves,
				})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no remediations in CloudEvents message")
	}
	return result, nil
}

func splitPackageName(pkgName string) (group, artifact string, err error) {
	parts := strings.SplitN(pkgName, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid package_name %q: expected group:artifact", pkgName)
	}
	return parts[0], parts[1], nil
}
