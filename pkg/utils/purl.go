package utils

import (
	"github.com/package-url/packageurl-go"
)

var languageMap = map[string]string{
	"npm":    "javascript",
	"maven":  "java",
	"pypi":   "python",
	"golang": "go",
	"cargo":  "rust",
}

type PURL struct {
	packageurl.PackageURL
}

func ParsePURL(purl string) *PURL {
	parsed, err := packageurl.FromString(purl)
	if err != nil {
		return nil
	}

	return &PURL{PackageURL: parsed}
}

func (p *PURL) FullName() string {
	if p.Namespace == "" {
		return p.Name
	}
	if p.Type == "maven" {
		return p.Namespace + ":" + p.Name
	}
	return p.Namespace + "/" + p.Name
}

func (p *PURL) Language() *string {
	mapped := languageMap[p.Type]
	if mapped == "" {
		return nil
	}
	return &mapped
}
