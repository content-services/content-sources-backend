package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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
func parsePOMs(r io.Reader) ([]Package, error) {
	projects, err := splitPOMs(r)
	if err != nil {
		return nil, err
	}

	var packages []Package
	parseErrors := make([]error, 0, len(projects))
	for i, project := range projects {
		parsed, err := parsePOM(bytes.NewReader(project))
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

// parsePOM uses git-pkgs/pom to parse the file, then resolve parents and BOM imports.
func parsePOM(r io.Reader) ([]Package, error) {
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

	ep, err := gitpom.NewResolver(pomFetcher).ResolvePOM(ctx, parsed, gitpom.Options{})
	if err != nil {
		return nil, wrapParse("POM", err)
	}
	return packagesFromEffective(parsed, ep), nil
}

func packagesFromEffective(parsed *gitpom.POM, ep *gitpom.EffectivePOM) []Package {
	collector := newPOMPackageCollector()

	for _, d := range ep.Dependencies {
		if strings.EqualFold(d.Scope, "import") {
			continue
		}
		ver := d.Version
		if d.Resolution != gitpom.Resolved {
			ver = ""
		}
		collector.add(d.GroupID, d.ArtifactID, ver)
	}

	managed := sortedManagedDependencies(ep.DependencyManagement)
	effectiveIdentities := make(map[string]struct{}, len(managed))
	for _, d := range managed {
		effectiveIdentities[managedDependencyIdentity(d)] = struct{}{}
		collector.add(d.GroupID, d.ArtifactID, d.Version)
	}

	for _, d := range parsed.DependencyManagement.Dependencies {
		d = interpolateManagedDependency(d, ep.Properties)
		if _, ok := effectiveIdentities[managedDependencyIdentity(d)]; ok {
			continue
		}
		collector.add(d.GroupID, d.ArtifactID, d.Version)
	}
	return collector.packages
}

type pomPackageCollector struct {
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

func sortedManagedDependencies(managed map[string]gitpom.Dep) []gitpom.Dep {
	keys := make([]string, 0, len(managed))
	for key := range managed {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	dependencies := make([]gitpom.Dep, 0, len(keys))
	for _, key := range keys {
		dependencies = append(dependencies, managed[key])
	}
	return dependencies
}

func managedDependencyIdentity(d gitpom.Dep) string {
	depType := d.Type
	if depType == "" {
		depType = "jar"
	}
	return d.GroupID + "|" + d.ArtifactID + "|" + depType + "|" + d.Classifier
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
