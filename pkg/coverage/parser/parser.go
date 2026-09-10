package parser

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	EcosystemJava   = "Java"
	EcosystemPython = "Python"
)

const (
	FormatCSV          = "csv"
	FormatRequirements = "requirements.txt"
	FormatPOM          = "pom.xml"
	FormatCycloneDX    = "CycloneDX"
	FormatSPDX         = "SPDX"
)

type Package struct {
	Ecosystem string
	Name      string
	Version   string
	Namespace string
}

type ParseResult struct {
	Packages    []Package
	InputFormat string
}

// Parse detects the format of the manifest file and extracts package metadata.
func Parse(filename string, r io.Reader) (*ParseResult, error) {
	br := bufio.NewReader(r)
	discardUTF8BOM(br)

	format, err := detectFormat(filename, br)
	if err != nil {
		return nil, err
	}

	var packages []Package
	switch format {
	case FormatCSV:
		packages, err = parseCSV(br)
	case FormatRequirements:
		packages, err = parseRequirements(br)
	case FormatPOM:
		packages, err = parsePOM(br)
	case FormatCycloneDX:
		packages, err = parseCycloneDX(br)
	case FormatSPDX:
		packages, err = parseSPDX(br)
	default:
		return nil, fmt.Errorf("unsupported manifest format for file %q", filename)
	}
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Packages:    deduplicate(packages),
		InputFormat: format,
	}, nil
}

// detectFormat uses the filename when it is unambiguous, otherwise peeks at the stream to sniff CycloneDX vs SPDX.
func detectFormat(filename string, r *bufio.Reader) (string, error) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".csv"):
		return FormatCSV, nil
	case strings.HasSuffix(lower, "requirements.txt"):
		return FormatRequirements, nil
	case path.Base(lower) == "pom.xml" || hasAnySuffix(lower, ".pom", "pom.xml"):
		return FormatPOM, nil
	case hasAnySuffix(lower, ".cdx.json", ".cdx.xml") ||
		path.Base(lower) == "bom.json" || path.Base(lower) == "bom.xml":
		return FormatCycloneDX, nil
	case hasAnySuffix(lower, ".spdx.json", ".spdx.tag", ".spdx"):
		return FormatSPDX, nil
	}

	if sniffed, err := sniffSBOMFormat(r); err != nil {
		return "", err
	} else if sniffed != "" {
		return sniffed, nil
	}

	return "", fmt.Errorf("unsupported manifest format for file %q", filename)
}

// deduplicate keeps the first occurrence of each ecosystem/namespace/name/version tuple.
func deduplicate(pkgs []Package) []Package {
	seen := make(map[string]struct{}, len(pkgs))
	result := make([]Package, 0, len(pkgs))
	for _, p := range pkgs {
		key := p.Ecosystem + "|" + p.Namespace + "|" + p.Name + "|" + p.Version
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, p)
	}
	return result
}
