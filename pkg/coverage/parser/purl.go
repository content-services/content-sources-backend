package parser

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/utils"
)

// parsePURL converts a Package URL into a coverage package.
// Invalid PURLs return nil. Known types get a display name; all others use the PURL type with the first letter capitalized and the rest lowercased.
func parsePURL(raw string) *Package {
	parsed := utils.ParsePURL(strings.TrimSpace(raw))
	if parsed == nil {
		return nil
	}

	return &Package{
		Ecosystem: ecosystemFromPURLType(parsed.Type),
		Name:      parsed.Name,
		Version:   parsed.Version,
		Namespace: parsed.Namespace,
	}
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
