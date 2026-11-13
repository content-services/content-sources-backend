package sonatype_bridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBOMCoordinates(t *testing.T) {
	rem := Remediation{GroupID: "org.springframework", ArtifactID: "spring-core", Version: "5.3.18.rhlw-00010"}
	sbom := []byte(`{
	  "bomFormat": "CycloneDX",
	  "specVersion": "1.7",
	  "metadata": {"timestamp": "2026-11-04T17:26:03Z"},
	  "components": [{
	    "type": "library",
	    "bom-ref": "pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00010",
	    "group": "org.springframework",
	    "name": "spring-core",
	    "version": "5.3.18.rhlw-00010",
	    "purl": "pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00010?repository_url=https://packages.redhat.com/lightwell/java/remediated/&type=jar"
	  }]
	}`)
	coords, err := parseBOMCoordinates(sbom, rem)
	require.NoError(t, err)
	assert.Equal(t, bomCoordinates{
		Ecosystem: "maven",
		Namespace: "org.springframework",
		Name:      "spring-core",
		Version:   "5.3.18.rhlw-00010",
	}, coords)
	assert.Equal(t, "packages/maven/org.springframework/spring-core/5.3.18.rhlw-00010", bundleBasePath(coords))
}

func TestParseBOMCoordinates_ComponentsMultipleMatchByGAV(t *testing.T) {
	rem := Remediation{GroupID: "org.springframework", ArtifactID: "spring-core", Version: "5.3.18.rhlw-00010"}
	sbom := []byte(`{"components":[
	  {"group":"org.springframework","name":"spring-core","version":"5.3.18.rhlw-00010","purl":"pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00010"},
	  {"group":"commons-io","name":"commons-io","version":"2.11.0","purl":"pkg:maven/commons-io/commons-io@2.11.0"}
	]}`)
	coords, err := parseBOMCoordinates(sbom, rem)
	require.NoError(t, err)
	assert.Equal(t, "spring-core", coords.Name)
}

func TestParseBOMCoordinates_GroupWinsOverPurlNamespace(t *testing.T) {
	rem := Remediation{GroupID: "org.springframework", ArtifactID: "spring-core", Version: "1.0"}
	sbom := []byte(`{"components":[{"group":"org.springframework","name":"spring-core","version":"1.0","purl":"pkg:maven/org.other/spring-core@1.0"}]}`)
	coords, err := parseBOMCoordinates(sbom, rem)
	require.NoError(t, err)
	assert.Equal(t, "org.springframework", coords.Namespace)
}

func TestParseBOMCoordinates_EmptyGroupOmitsNamespace(t *testing.T) {
	rem := Remediation{ArtifactID: "flask", Version: "3.0.3"}
	sbom := []byte(`{"components":[{"name":"flask","version":"3.0.3","purl":"pkg:pypi/flask@3.0.3"}]}`)
	coords, err := parseBOMCoordinates(sbom, rem)
	require.NoError(t, err)
	assert.Equal(t, "packages/pypi/flask/3.0.3", bundleBasePath(coords))
}

func TestParseBOMCoordinates_Errors(t *testing.T) {
	rem := Remediation{GroupID: "g", ArtifactID: "n", Version: "1"}
	tests := map[string]struct {
		sbom string
		want string
	}{
		"invalid json":      {`not json`, "parse SBOM"},
		"no components":     {`{"bomFormat":"CycloneDX"}`, "SBOM has no components"},
		"missing purl":      {`{"components":[{"group":"g","name":"n","version":"1"}]}`, "purl is missing"},
		"missing name":      {`{"components":[{"group":"g","version":"1","purl":"pkg:maven/g/n@1"}]}`, "name is missing"},
		"missing version":   {`{"components":[{"group":"g","name":"n","purl":"pkg:maven/g/n@1"}]}`, "version is missing"},
		"bad purl":          {`{"components":[{"group":"g","name":"n","version":"1","purl":"maven:g:n"}]}`, "purl"},
		"traversal":         {`{"components":[{"group":"g","name":"..","version":"1","purl":"pkg:maven/g/n@1"}]}`, "not a valid path segment"},
		"slash in group":    {`{"components":[{"group":"g/h","name":"n","version":"1","purl":"pkg:maven/g/n@1"}]}`, "not a valid path segment"},
		"no gav match":      {`{"components":[{"group":"o","name":"other","version":"9","purl":"pkg:maven/o/other@9"},{"group":"p","name":"third","version":"8","purl":"pkg:maven/p/third@8"}]}`, "no entry in components"},
		"ambiguous matches": {`{"components":[{"group":"g","name":"n","version":"1","purl":"pkg:maven/g/n@1"},{"name":"n","version":"1","purl":"pkg:maven/g/n@1"}]}`, "2 entries in components"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseBOMCoordinates([]byte(tc.sbom), rem)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateCoordinates(t *testing.T) {
	rem := Remediation{GroupID: "org.springframework", ArtifactID: "spring-core", Version: "5.3.18.rhlw-00003"}
	ok := bomCoordinates{Ecosystem: "maven", Namespace: "org.springframework", Name: "spring-core", Version: "5.3.18.rhlw-00003"}
	assert.NoError(t, validateCoordinates(ok, rem))

	noGroup := ok
	noGroup.Namespace = ""
	assert.NoError(t, validateCoordinates(noGroup, rem), "empty group is not compared")

	badName := ok
	badName.Name = "spring-beans"
	assert.ErrorContains(t, validateCoordinates(badName, rem), "name")

	badVersion := ok
	badVersion.Version = "5.3.18.rhlw-00004"
	assert.ErrorContains(t, validateCoordinates(badVersion, rem), "version")

	badGroup := ok
	badGroup.Namespace = "org.other"
	assert.ErrorContains(t, validateCoordinates(badGroup, rem), "group")
}
