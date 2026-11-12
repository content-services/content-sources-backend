package sonatype_bridge

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/package-url/packageurl-go"
)

const packagesRoot = "packages"

// bomCoordinates identifies a component in the partner bucket layout:
// packages/<ecosystem>/<namespace>/<name>/<version>/.
type bomCoordinates struct {
	Ecosystem string
	Namespace string
	Name      string
	Version   string
}

type bomComponent struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Purl    string `json:"purl"`
}

type bomDocument struct {
	Components []bomComponent `json:"components"`
}

// parseBOMCoordinates reads the component a CycloneDX SBOM describes from its
// top-level components array. The ecosystem is the purl type; namespace, name
// and version come from the component's group, name and version fields (group
// wins over the purl namespace).
func parseBOMCoordinates(sbom []byte, rem Remediation) (bomCoordinates, error) {
	var doc bomDocument
	if err := json.Unmarshal(sbom, &doc); err != nil {
		return bomCoordinates{}, fmt.Errorf("parse SBOM: %w", err)
	}
	comp, err := selectBOMComponent(doc.Components, rem)
	if err != nil {
		return bomCoordinates{}, err
	}
	if comp.Purl == "" {
		return bomCoordinates{}, fmt.Errorf("SBOM component purl is missing")
	}
	if comp.Name == "" {
		return bomCoordinates{}, fmt.Errorf("SBOM component name is missing")
	}
	if comp.Version == "" {
		return bomCoordinates{}, fmt.Errorf("SBOM component version is missing")
	}
	purl, err := packageurl.FromString(comp.Purl)
	if err != nil {
		return bomCoordinates{}, fmt.Errorf("SBOM component purl %q: %w", comp.Purl, err)
	}
	coords := bomCoordinates{
		Ecosystem: strings.ToLower(purl.Type),
		Namespace: comp.Group,
		Name:      comp.Name,
		Version:   comp.Version,
	}
	for label, seg := range map[string]string{
		"ecosystem": coords.Ecosystem,
		"name":      coords.Name,
		"version":   coords.Version,
	} {
		if err := validatePathSegment(label, seg); err != nil {
			return bomCoordinates{}, err
		}
	}
	if coords.Namespace != "" {
		if err := validatePathSegment("namespace", coords.Namespace); err != nil {
			return bomCoordinates{}, err
		}
	}
	return coords, nil
}

// selectBOMComponent picks the subject from the components array: a single
// entry is taken as-is; otherwise the entry matching the notification
// coordinates must be unique.
func selectBOMComponent(components []bomComponent, rem Remediation) (bomComponent, error) {
	if len(components) == 0 {
		return bomComponent{}, fmt.Errorf("SBOM has no components")
	}
	if len(components) == 1 {
		return components[0], nil
	}
	var match bomComponent
	matches := 0
	for _, c := range components {
		if c.Name == rem.ArtifactID && c.Version == rem.Version && (c.Group == "" || c.Group == rem.GroupID) {
			match = c
			matches++
		}
	}
	switch matches {
	case 1:
		return match, nil
	case 0:
		return bomComponent{}, fmt.Errorf("SBOM has no entry in components matching %s", gavKey(rem))
	default:
		return bomComponent{}, fmt.Errorf("SBOM has %d entries in components matching %s", matches, gavKey(rem))
	}
}

func validatePathSegment(label, seg string) error {
	if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `/\`) {
		return fmt.Errorf("SBOM %s %q is not a valid path segment", label, seg)
	}
	return nil
}

// validateCoordinates ensures the SBOM describes the artifact named in the
// Kafka notification, so a bundle is never uploaded under the wrong path.
func validateCoordinates(coords bomCoordinates, rem Remediation) error {
	if coords.Name != rem.ArtifactID {
		return fmt.Errorf("SBOM name %q does not match notification artifact %q", coords.Name, rem.ArtifactID)
	}
	if coords.Version != rem.Version {
		return fmt.Errorf("SBOM version %q does not match notification version %q", coords.Version, rem.Version)
	}
	if coords.Namespace != "" && coords.Namespace != rem.GroupID {
		return fmt.Errorf("SBOM group %q does not match notification group %q", coords.Namespace, rem.GroupID)
	}
	return nil
}

// bundleBasePath returns packages/<ecosystem>[/<namespace>]/<name>/<version>.
func bundleBasePath(coords bomCoordinates) string {
	parts := []string{packagesRoot, coords.Ecosystem}
	if coords.Namespace != "" {
		parts = append(parts, coords.Namespace)
	}
	parts = append(parts, coords.Name, coords.Version)
	return strings.Join(parts, "/")
}
