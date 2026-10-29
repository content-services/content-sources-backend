package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gitpom "github.com/git-pkgs/pom"
)

const pomResolveTimeout = 2 * time.Minute

// pomFetcher loads parent POMs and imported BOMs. Tests replace it with an offline stub.
var pomFetcher gitpom.Fetcher = newHTTPPOMFetcher()

func newHTTPPOMFetcher() gitpom.Fetcher {
	f := gitpom.NewHTTPFetcher("")
	f.Client = &http.Client{Timeout: 30 * time.Second}
	return gitpom.NewCachingFetcher(f)
}

// splitPOMs returns a single POM unchanged or the Maven project children of a projects wrapper.
func splitPOMs(r io.Reader) ([][]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	root, err := pomRoot(dec)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	switch root.Name.Local {
	case "project":
		if err := dec.Skip(); err != nil {
			return nil, wrapParse("POM", err)
		}
		if err := requireXMLEnd(dec); err != nil {
			return nil, wrapParse("POM", err)
		}
		return [][]byte{data}, nil
	case "projects":
		var projects [][]byte
		for {
			start := dec.InputOffset()
			tok, err := dec.Token()
			if err != nil {
				return nil, wrapParse("POM", err)
			}
			switch element := tok.(type) {
			case xml.StartElement:
				if err := dec.Skip(); err != nil {
					return nil, wrapParse("POM", err)
				}
				if element.Name.Local != "project" {
					continue
				}
				project := data[start:dec.InputOffset()]
				if element.Name.Space == mavenPOMNamespace || isPOM(string(project)) {
					projects = append(projects, project)
				}
			case xml.EndElement:
				if err := requireXMLEnd(dec); err != nil {
					return nil, wrapParse("POM", err)
				}
				if len(projects) == 0 {
					return nil, wrapParse("POM", errors.New("projects wrapper contains no Maven projects"))
				}
				return projects, nil
			}
		}
	default:
		return nil, wrapParse("POM", fmt.Errorf("unexpected root element %q", root.Name.Local))
	}
}

func pomRoot(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return xml.StartElement{}, errors.New("XML document has no root element")
			}
			return xml.StartElement{}, err
		}
		switch token := tok.(type) {
		case xml.StartElement:
			return token, nil
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return xml.StartElement{}, errors.New("unexpected text before root element")
			}
		}
	}
}

func requireXMLEnd(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch token := tok.(type) {
		case xml.StartElement, xml.EndElement:
			return errors.New("XML document contains multiple root elements")
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return errors.New("unexpected text after root element")
			}
		}
	}
}

// parsePOMs parses every project in a POM document and keeps results from successful projects.
func parsePOMs(r io.Reader, skipped *int) ([]Package, error) {
	projects, err := splitPOMs(r)
	if err != nil {
		return nil, err
	}

	var packages []Package
	parseErrors := make([]error, 0, len(projects))
	for i, project := range projects {
		parsed, err := parsePOM(bytes.NewReader(project), skipped)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("project %d: %w", i+1, err))
			continue
		}
		packages = append(packages, parsed...)
	}
	if len(parseErrors) == len(projects) {
		return nil, errors.Join(parseErrors...)
	}
	return packages, nil
}

// parsePOM parses the file and resolves parent POMs and imported BOMs so an omitted
// dependency version can be filled in. Packages that exist only on a parent are not reported.
func parsePOM(r io.Reader, skipped *int) ([]Package, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	parsed, err := gitpom.ParsePOM(data)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pomResolveTimeout)
	defer cancel()

	ep, err := gitpom.NewResolver(pomFetcher).ResolvePOM(ctx, pomWithoutEmptyManagedVersions(parsed), gitpom.Options{})
	if err != nil {
		return nil, wrapParse("POM", err)
	}
	return packagesFromEffective(parsed, ep, skipped), nil
}

// pomWithoutEmptyManagedVersions copies parsed and drops dependencyManagement entries
// that omit a version. The resolver replaces a parent entry wholesale, so leaving an
// empty child entry in place would discard the parent version. Import-scoped BOM
// entries are kept.
func pomWithoutEmptyManagedVersions(parsed *gitpom.POM) *gitpom.POM {
	clone := *parsed
	clone.DependencyManagement.Dependencies = omitEmptyManagedVersions(parsed.DependencyManagement.Dependencies)
	if len(parsed.Profiles) == 0 {
		return &clone
	}
	clone.Profiles = make([]gitpom.Profile, len(parsed.Profiles))
	for i, profile := range parsed.Profiles {
		profile.DependencyManagement.Dependencies = omitEmptyManagedVersions(profile.DependencyManagement.Dependencies)
		clone.Profiles[i] = profile
	}
	return &clone
}

func omitEmptyManagedVersions(deps []gitpom.Dep) []gitpom.Dep {
	if len(deps) == 0 {
		return deps
	}
	filtered := make([]gitpom.Dep, 0, len(deps))
	for _, dep := range deps {
		if strings.TrimSpace(dep.Version) == "" && !strings.EqualFold(dep.Scope, "import") {
			continue
		}
		filtered = append(filtered, dep)
	}
	return filtered
}

// packagesFromEffective reports dependencies and dependencyManagement entries declared
// on the uploaded POM. Versions come from the effective model, so a parent or imported
// BOM can fill an omitted version, but packages declared only on a parent are left out.
func packagesFromEffective(parsed *gitpom.POM, ep *gitpom.EffectivePOM, skipped *int) []Package {
	collector := newPOMPackageCollector()
	collector.skipped = skipped
	resolved := indexResolvedDependencies(ep.Dependencies)
	active := activeProfileIDs(ep)

	for _, dep := range declaredDependencies(parsed, active) {
		if strings.EqualFold(dep.Scope, "import") {
			continue
		}
		dep = interpolateManagedDependency(dep, ep.Properties)
		match, ok := lookupResolvedDependency(resolved, dep)
		if !ok {
			collector.add(dep.GroupID, dep.ArtifactID, dep.Version)
			continue
		}
		if strings.EqualFold(match.Scope, "import") {
			continue
		}
		version := ""
		if match.Resolution == gitpom.Resolved {
			version = match.Version
		}
		collector.add(match.GroupID, match.ArtifactID, version)
	}

	for _, dep := range declaredDependencyManagement(parsed, active) {
		dep = interpolateManagedDependency(dep, ep.Properties)
		dep = applyEffectiveManagedVersion(dep, ep.DependencyManagement)
		collector.add(dep.GroupID, dep.ArtifactID, dep.Version)
	}
	return collector.packages
}

func declaredDependencies(parsed *gitpom.POM, active map[string]struct{}) []gitpom.Dep {
	deps := append([]gitpom.Dep{}, parsed.Dependencies...)
	for _, profile := range parsed.Profiles {
		if _, ok := active[profile.ID]; !ok {
			continue
		}
		deps = append(deps, profile.Dependencies...)
	}
	return deps
}

func declaredDependencyManagement(parsed *gitpom.POM, active map[string]struct{}) []gitpom.Dep {
	deps := append([]gitpom.Dep{}, parsed.DependencyManagement.Dependencies...)
	for _, profile := range parsed.Profiles {
		if _, ok := active[profile.ID]; !ok {
			continue
		}
		deps = append(deps, profile.DependencyManagement.Dependencies...)
	}
	return deps
}

func activeProfileIDs(ep *gitpom.EffectivePOM) map[string]struct{} {
	active := make(map[string]struct{}, len(ep.ActiveProfiles))
	for _, id := range ep.ActiveProfiles {
		active[id] = struct{}{}
	}
	return active
}

func indexResolvedDependencies(deps []gitpom.ResolvedDep) map[string]gitpom.ResolvedDep {
	index := make(map[string]gitpom.ResolvedDep, len(deps))
	for _, dep := range deps {
		key := pomManagementKey(dep.GroupID, dep.ArtifactID, dep.Type, dep.Classifier)
		if existing, ok := index[key]; ok && existing.Resolution == gitpom.Resolved && dep.Resolution != gitpom.Resolved {
			continue
		}
		index[key] = dep
	}
	return index
}

func lookupResolvedDependency(index map[string]gitpom.ResolvedDep, dep gitpom.Dep) (gitpom.ResolvedDep, bool) {
	if resolved, ok := index[pomManagementKey(dep.GroupID, dep.ArtifactID, dep.Type, dep.Classifier)]; ok {
		return resolved, true
	}
	if dep.Type == "" && dep.Classifier == "" {
		return gitpom.ResolvedDep{}, false
	}
	resolved, ok := index[pomManagementKey(dep.GroupID, dep.ArtifactID, "", "")]
	return resolved, ok
}

func applyEffectiveManagedVersion(dep gitpom.Dep, managed map[string]gitpom.Dep) gitpom.Dep {
	md, ok := lookupEffectiveManaged(managed, dep)
	if !ok {
		return dep
	}
	if md.GroupID != "" {
		dep.GroupID = md.GroupID
	}
	if md.ArtifactID != "" {
		dep.ArtifactID = md.ArtifactID
	}
	if concretePOMVersion(md.Version) || (dep.Version == "" && md.Version != "") {
		dep.Version = md.Version
	}
	return dep
}

func lookupEffectiveManaged(managed map[string]gitpom.Dep, dep gitpom.Dep) (gitpom.Dep, bool) {
	if len(managed) == 0 {
		return gitpom.Dep{}, false
	}
	if md, ok := managed[pomManagementKey(dep.GroupID, dep.ArtifactID, dep.Type, dep.Classifier)]; ok {
		return md, true
	}
	if dep.Type == "" && dep.Classifier == "" {
		return gitpom.Dep{}, false
	}
	md, ok := managed[pomManagementKey(dep.GroupID, dep.ArtifactID, "", "")]
	return md, ok
}

// pomManagementKey matches git-pkgs/pom's unexported dependencyManagement identity:
// groupId:artifactId:type:classifier, with an empty type treated as jar.
func pomManagementKey(groupID, artifactID, depType, classifier string) string {
	if depType == "" {
		depType = "jar"
	}
	return groupID + ":" + artifactID + ":" + depType + ":" + classifier
}

func concretePOMVersion(version string) bool {
	return version != "" && !strings.Contains(version, "${")
}

type pomPackageCollector struct {
	skipped            *int
	packages           []Package
	seenGAVs           map[string]struct{}
	versionedGAs       map[string]struct{}
	unversionedIndexes map[string]int
}

func newPOMPackageCollector() *pomPackageCollector {
	return &pomPackageCollector{
		seenGAVs:           make(map[string]struct{}),
		versionedGAs:       make(map[string]struct{}),
		unversionedIndexes: make(map[string]int),
	}
}

func (c *pomPackageCollector) add(groupID, artifactID, version string) {
	if groupID == "" || artifactID == "" ||
		strings.Contains(groupID, "${") || strings.Contains(artifactID, "${") {
		if c.skipped != nil {
			*c.skipped++
		}
		return
	}
	if strings.Contains(version, "${") {
		version = ""
	}

	gaKey := groupID + "|" + artifactID
	if version == "" {
		if _, ok := c.versionedGAs[gaKey]; ok {
			return
		}
	}

	gavKey := gaKey + "|" + version
	if _, ok := c.seenGAVs[gavKey]; ok {
		return
	}

	pkg := Package{
		Ecosystem: EcosystemJava,
		Namespace: groupID,
		Name:      artifactID,
		Version:   version,
	}
	if version == "" {
		c.unversionedIndexes[gaKey] = len(c.packages)
	} else {
		c.versionedGAs[gaKey] = struct{}{}
		if index, ok := c.unversionedIndexes[gaKey]; ok {
			delete(c.seenGAVs, gaKey+"|")
			delete(c.unversionedIndexes, gaKey)
			c.seenGAVs[gavKey] = struct{}{}
			c.packages[index] = pkg
			return
		}
	}

	c.seenGAVs[gavKey] = struct{}{}
	c.packages = append(c.packages, pkg)
}

func interpolateManagedDependency(d gitpom.Dep, properties map[string]string) gitpom.Dep {
	d.GroupID = interpolatePOMValue(d.GroupID, properties)
	d.ArtifactID = interpolatePOMValue(d.ArtifactID, properties)
	d.Version = interpolatePOMValue(d.Version, properties)
	d.Type = interpolatePOMValue(d.Type, properties)
	d.Classifier = interpolatePOMValue(d.Classifier, properties)
	return d
}

func interpolatePOMValue(value string, properties map[string]string) string {
	for range len(properties) {
		interpolated := value
		for name, replacement := range properties {
			interpolated = strings.ReplaceAll(interpolated, "${"+name+"}", replacement)
		}
		if interpolated == value {
			break
		}
		value = interpolated
	}
	return value
}
