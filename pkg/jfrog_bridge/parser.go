package jfrog_bridge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Remediation is the internal representation of a single package release
// extracted from either the full Kafka envelope or the simplified format.
type Remediation struct {
	GroupID     string
	ArtifactID  string
	Version     string
	BaseVersion string
	CVEsFixed   []string
}

var rhlwSuffix = regexp.MustCompile(`\.rhlw-\d+$`)

func stripRHLWSuffix(version string) string {
	return rhlwSuffix.ReplaceAllString(version, "")
}

// ParseRemediations accepts both the full NotificationAction envelope and
// the simplified format. Returns one Remediation per (group, artifact, version).
func ParseRemediations(data []byte) ([]Remediation, error) {
	remediations, err := parseFullEnvelope(data)
	if err == nil && len(remediations) > 0 {
		return remediations, nil
	}
	return parseSimplified(data)
}

// fullEnvelope mirrors the NotificationAction structure from pkg/event.
type fullEnvelope struct {
	Application string `json:"application"`
	EventType   string `json:"event_type"`
	Events      []struct {
		Payload json.RawMessage `json:"payload"`
	} `json:"events"`
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

func parseFullEnvelope(data []byte) ([]Remediation, error) {
	var env fullEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Application == "" || len(env.Events) == 0 {
		return nil, fmt.Errorf("not a full envelope")
	}

	var result []Remediation
	for _, evt := range env.Events {
		var p payloadFull
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
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
		return nil, fmt.Errorf("no remediations found in envelope")
	}
	return result, nil
}

type simplifiedMessage struct {
	PackageName string `json:"package_name"`
	Releases    []struct {
		Name      string   `json:"name"`
		CVEsFixed []string `json:"cves_fixed"`
	} `json:"releases"`
}

func parseSimplified(data []byte) ([]Remediation, error) {
	var msg simplifiedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse simplified message: %w", err)
	}
	if msg.PackageName == "" {
		return nil, fmt.Errorf("missing package_name")
	}
	group, artifact, err := splitPackageName(msg.PackageName)
	if err != nil {
		return nil, err
	}
	var result []Remediation
	for _, rel := range msg.Releases {
		result = append(result, Remediation{
			GroupID:     group,
			ArtifactID:  artifact,
			Version:     rel.Name,
			BaseVersion: stripRHLWSuffix(rel.Name),
			CVEsFixed:   rel.CVEsFixed,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no releases in simplified message")
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
