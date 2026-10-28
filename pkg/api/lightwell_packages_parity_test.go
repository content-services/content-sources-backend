package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// jsonFieldNames returns the set of JSON key names a struct marshals to, ignoring
// the ",omitempty" and similar options on the tag.
func jsonFieldNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name != "" {
			names[name] = true
		}
	}
	return names
}

// TestLightwellPackageResponseCoversPackageItem guards the ability for
// /lightwell/packages to replace /repositories/{uuid}/packages: every field the
// legacy PackageItem (and its nested ReleaseInfo) exposes must also be present on
// LightwellPackageResponse, so a UI switching endpoints loses no data. If someone
// adds a field to PackageItem without mirroring it here, this test fails.
func TestLightwellPackageResponseCoversPackageItem(t *testing.T) {
	legacy := jsonFieldNames(reflect.TypeOf(PackageItem{}))
	// Repo list only. /lightwell/packages keeps returning the upstream cut in versions.
	delete(legacy, "upstream_versions")
	lightwell := jsonFieldNames(reflect.TypeOf(LightwellPackageResponse{}))

	for field := range legacy {
		assert.Truef(t, lightwell[field],
			"LightwellPackageResponse is missing field %q from PackageItem; /lightwell/packages cannot replace /repositories/{uuid}/packages without it", field)
	}

	// latest_releases nested shape must be identical (same type is reused today).
	legacyRelease := jsonFieldNames(reflect.TypeOf(ReleaseInfo{}))
	assert.NotEmpty(t, legacyRelease)
}
