package parser

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/utils"
)

// parsePURL converts supported Package URLs into coverage packages.
func parsePURL(raw string) *Package {
	parsed := utils.ParsePURL(strings.TrimSpace(raw))
	if parsed == nil {
		return nil
	}

	ecosystem := map[string]string{
		"maven": EcosystemJava,
		"pypi":  EcosystemPython,
	}[parsed.Type]
	if ecosystem == "" {
		return nil
	}

	return &Package{
		Ecosystem: ecosystem,
		Name:      parsed.Name,
		Version:   parsed.Version,
		Namespace: parsed.Namespace,
	}
}
