package coords

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
)

func TestBuildPURL(t *testing.T) {
	cases := []struct{ ct, group, name, version, want string }{
		{config.ContentTypeMaven, "org.apache", "commons", "1.0", "pkg:maven/org.apache/commons@1.0"},
		{config.ContentTypePython, "", "requests", "2.0", "pkg:pypi/requests@2.0"},
		{config.ContentTypeNpm, "-", "left-pad", "1.0.0", "pkg:npm/left-pad@1.0.0"},
		{config.ContentTypeNpm, "@types", "node", "20.0.0", "pkg:npm/%40types/node@20.0.0"},
	}
	for _, c := range cases {
		if got := BuildPURL(c.ct, c.group, c.name, c.version); got != c.want {
			t.Errorf("BuildPURL(%q,%q,%q,%q)=%q want %q", c.ct, c.group, c.name, c.version, got, c.want)
		}
	}
}

func TestBuildCoordinates(t *testing.T) {
	cases := []struct{ ct, group, name, want string }{
		{config.ContentTypeMaven, "org.apache", "commons", "org.apache:commons"},
		{config.ContentTypePython, "", "requests", "requests"},
		{config.ContentTypeNpm, "-", "left-pad", "left-pad"},
		{config.ContentTypeNpm, "@types", "node", "@types/node"},
	}
	for _, c := range cases {
		if got := BuildCoordinates(c.ct, c.group, c.name); got != c.want {
			t.Errorf("BuildCoordinates(%q,%q,%q)=%q want %q", c.ct, c.group, c.name, got, c.want)
		}
	}
}
