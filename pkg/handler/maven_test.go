package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMavenGroupPath(t *testing.T) {
	assert.Equal(t, "org/apache/commons", mavenGroupPath("org.apache.commons"))
}

func TestMavenCentralPomPath(t *testing.T) {
	path := mavenCentralPomPath("avalon-util", "avalon-util-exception", "1.0.0")
	assert.Equal(t, "/avalon-util/avalon-util-exception/1.0.0/avalon-util-exception-1.0.0.pom", path)
}

func TestMavenCentralPomURL(t *testing.T) {
	url := mavenCentralPomURL("commons-io", "commons-io", "2.11.0")
	assert.Equal(t, "https://repo.maven.apache.org/maven2/commons-io/commons-io/2.11.0/commons-io-2.11.0.pom", url)
}

func TestStripLightwellVersionSuffix(t *testing.T) {
	assert.Equal(t, "3.15.0", stripLightwellVersionSuffix("3.15.0.rhlw-3001"))
	assert.Equal(t, "42.0.8", stripLightwellVersionSuffix("42.0.8.rhlw-00003"))
	assert.Equal(t, "1.0.0", stripLightwellVersionSuffix("1.0.0"))
}

func TestParsePom(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project>
  <description>Utility classes for exception handling</description>
  <url>https://example.com/project</url>
  <organization>
    <name>FasterXML</name>
  </organization>
  <licenses>
    <license>
      <name>Apache License 2.0</name>
    </license>
  </licenses>
</project>`)

	metadata, err := parsePom(data)
	require.NoError(t, err)
	require.NotNil(t, metadata.Summary)
	require.NotNil(t, metadata.License)
	require.NotNil(t, metadata.ProjectURL)
	require.NotNil(t, metadata.Author)
	assert.Equal(t, "Utility classes for exception handling", *metadata.Summary)
	assert.Equal(t, "Apache License 2.0", *metadata.License)
	assert.Equal(t, "https://example.com/project", *metadata.ProjectURL)
	assert.Equal(t, "FasterXML", *metadata.Author)
}

func TestParsePomEmptyFields(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?><project></project>`)

	metadata, err := parsePom(data)
	require.NoError(t, err)
	assert.Nil(t, metadata.Summary)
	assert.Nil(t, metadata.License)
	assert.Nil(t, metadata.ProjectURL)
	assert.Nil(t, metadata.Author)
}

func TestFetchMavenCentralMetadataResolvesParentLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/commons-io/commons-io/2.9.0/commons-io-2.9.0.pom":
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<project>
  <parent>
    <groupId>org.apache.commons</groupId>
    <artifactId>commons-parent</artifactId>
    <version>52</version>
  </parent>
  <description>The Apache Commons IO library</description>
  <url>https://commons.apache.org/proper/commons-io/</url>
</project>`))
		case "/org/apache/commons/commons-parent/52/commons-parent-52.pom":
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<project>
  <organization>
    <name>The Apache Software Foundation</name>
  </organization>
  <licenses>
    <license>
      <name>Apache License, Version 2.0</name>
    </license>
  </licenses>
</project>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalBaseURL := mavenCentralBaseURL
	mavenCentralBaseURL = server.URL
	t.Cleanup(func() { mavenCentralBaseURL = originalBaseURL })

	metadata, err := fetchMavenCentralMetadata(context.Background(), server.Client(), "commons-io", "commons-io", "2.9.0")
	require.NoError(t, err)
	require.NotNil(t, metadata.Summary)
	require.NotNil(t, metadata.ProjectURL)
	require.NotNil(t, metadata.License)
	require.NotNil(t, metadata.Author)
	assert.Equal(t, "The Apache Commons IO library", *metadata.Summary)
	assert.Equal(t, "https://commons.apache.org/proper/commons-io/", *metadata.ProjectURL)
	assert.Equal(t, "Apache License, Version 2.0", *metadata.License)
	assert.Equal(t, "The Apache Software Foundation", *metadata.Author)
}

func TestIsMavenCentralPomNotFound(t *testing.T) {
	assert.True(t, isMavenCentralPomNotFound(fmt.Errorf("POM not found: https://example.com/foo.pom")))
	assert.False(t, isMavenCentralPomNotFound(fmt.Errorf("connection refused")))
	assert.False(t, isMavenCentralPomNotFound(nil))
}

func TestValidMavenCoordinates(t *testing.T) {
	assert.True(t, isValid("commons-io"))
	assert.True(t, isValid("org.apache.commons"))
	assert.True(t, isValid("2.11.0"))
	assert.True(t, isValid("3.15.0.rhlw-3001"))

	assert.False(t, isValid(""))
	assert.False(t, isValid("../../127.0.0.1"))
	assert.False(t, isValid("commons-io@evil.com"))
	assert.False(t, isValid("1.0.0/../../../"))
}

func TestRejectsInvalidCoordinates(t *testing.T) {
	_, err := fetchMavenCentralMetadata(context.Background(), nil, "../../127.0.0.1", "commons-io", "1.0.0")
	assert.Error(t, err)
}
