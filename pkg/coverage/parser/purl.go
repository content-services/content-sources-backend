package parser

import (
	"strings"

	"github.com/package-url/packageurl-go"
)

// parsePURL converts a Package URL into a coverage package.
// Invalid PURLs return nil. Known types get a display name; all others use the PURL type with the first letter capitalized and the rest lowercased.
func parsePURL(raw string) *Package {
	pkg, err := ParsePURL(raw)
	if err != nil {
		return nil
	}
	return &pkg
}

// ParsePURL parses a Package URL into a Package using the same ecosystem display
// names as coverage reports. Surrounding whitespace is ignored. It returns the
// Package URL parser's error when raw is not a valid PURL.
func ParsePURL(raw string) (Package, error) {
	parsed, err := packageurl.FromString(strings.TrimSpace(raw))
	if err != nil {
		return Package{}, err
	}
	return Package{
		Ecosystem: ecosystemFromPURLType(parsed.Type),
		Name:      parsed.Name,
		Version:   parsed.Version,
		Namespace: parsed.Namespace,
	}, nil
}

// purlTypeToEcosystem maps Package URL types onto the shared parser display names.
// maven/pypi/npm/golang/cargo follow pkg/utils/purl.go languageMap (title-cased); nuget is C#.
var purlTypeToEcosystem = map[string]string{
	"maven":  EcosystemJava,
	"pypi":   EcosystemPython,
	"npm":    EcosystemJavaScript,
	"nuget":  EcosystemCSharp,
	"golang": EcosystemGo,
	"cargo":  EcosystemRust,
}

func ecosystemFromPURLType(purlType string) string {
	if name, found := purlTypeToEcosystem[purlType]; found {
		return name
	}
	return capitalizeFirst(purlType)
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
