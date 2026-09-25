package coords

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
)

// BuildPURL returns the Package URL for a package version.
func BuildPURL(contentType, group, name, version string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("pkg:maven/%s/%s@%s", group, name, version)
	case config.ContentTypePython:
		return fmt.Sprintf("pkg:pypi/%s@%s", name, version)
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return fmt.Sprintf("pkg:npm/%s@%s", name, version)
		}
		scope := strings.TrimPrefix(group, "@")
		return fmt.Sprintf("pkg:npm/%%40%s/%s@%s", scope, name, version)
	default:
		return ""
	}
}

// BuildCoordinates returns the ecosystem coordinate string for a package.
func BuildCoordinates(contentType, group, name string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("%s:%s", group, name)
	case config.ContentTypePython:
		return name
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return name
		}
		return fmt.Sprintf("%s/%s", group, name)
	default:
		return ""
	}
}
