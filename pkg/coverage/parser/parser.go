package parser

import (
	"fmt"
	"io"
	"strings"
)

const (
	EcosystemJava   = "Java"
	EcosystemPython = "Python"
)

const (
	FormatCSV          = "csv"
	FormatRequirements = "requirements.txt"
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
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	format, err := detectFormat(filename)
	if err != nil {
		return nil, err
	}

	var packages []Package
	switch format {
	case FormatCSV:
		packages, err = parseCSV(data)
	case FormatRequirements:
		packages, err = parseRequirements(data)
	}
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Packages:    deduplicate(packages),
		InputFormat: format,
	}, nil
}

func detectFormat(filename string) (string, error) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".csv"):
		return FormatCSV, nil
	case strings.HasSuffix(lower, "requirements.txt"):
		return FormatRequirements, nil
	}
	return "", fmt.Errorf("unsupported manifest format for file %q", filename)
}

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
