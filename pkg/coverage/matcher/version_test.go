package matcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNearestHigherVersion(t *testing.T) {
	py := func(name, version, publishedAt string) Package {
		return Package{Ecosystem: EcosystemPython, Name: name, Version: version, PublishedAt: publishedAt}
	}
	java := func(namespace, name, version string) Package {
		return Package{Ecosystem: EcosystemJava, Namespace: namespace, Name: name, Version: version}
	}
	javaAt := func(namespace, name, version, publishedAt string) Package {
		return Package{Ecosystem: EcosystemJava, Namespace: namespace, Name: name, Version: version, PublishedAt: publishedAt}
	}

	for _, tc := range []struct {
		name     string
		catalog  []Package
		pkg      Package
		status   string
		versions []string
		nearest  string
		segment  string
	}{
		{
			name:     "exact clears nearest and keeps every catalog version",
			catalog:  []Package{py("flask", "1.4.2", ""), py("flask", "1.4.9", "")},
			pkg:      py("flask", "1.4.2", ""),
			status:   MatchStatusExact,
			versions: []string{"1.4.2", "1.4.9"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "same minor line",
			catalog:  []Package{py("flask", "1.4.0", ""), py("flask", "1.4.9", ""), py("flask", "1.5.0", "")},
			pkg:      py("flask", "1.4.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.4.0", "1.4.9", "1.5.0"},
			nearest:  "1.4.9",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "same major only",
			catalog:  []Package{py("flask", "1.7.0", "")},
			pkg:      py("flask", "1.4.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.7.0"},
			nearest:  "1.7.0",
			segment:  MatchSegmentMajor,
		},
		{
			name:     "different major",
			catalog:  []Package{py("flask", "2.0.0", ""), py("flask", "3.1.0", "")},
			pkg:      py("flask", "1.4.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"2.0.0", "3.1.0"},
			nearest:  "2.0.0",
			segment:  MatchSegmentNone,
		},
		{
			name:     "nothing higher",
			catalog:  []Package{py("flask", "1.3.0", ""), py("flask", "1.4.0", "")},
			pkg:      py("flask", "1.4.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.3.0", "1.4.0"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "java tag differs",
			catalog:  []Package{java("org.example", "lib", "1.4.2.9"), java("org.example", "lib", "1.4.3.0")},
			pkg:      java("org.example", "lib", "1.4.2.3"),
			status:   MatchStatusPartial,
			versions: []string{"1.4.2.9", "1.4.3.0"},
			nearest:  "1.4.2.9",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "trailing word sorts and compares by its numeric prefix",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "4.1.108.Final"), java("org.hibernate", "hibernate-core", "4.1.9.Final"), java("org.hibernate", "hibernate-core", "4.1.86.Final")},
			pkg:      java("org.hibernate", "hibernate-core", "4.1.80.Final"),
			status:   MatchStatusPartial,
			versions: []string{"4.1.9.Final", "4.1.86.Final", "4.1.108.Final"},
			nearest:  "4.1.86.Final",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "smallest higher numeric prefix wins",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "6.0.0.Final"), java("org.hibernate", "hibernate-core", "5.6.15.RELEASE")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.14.Final"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.RELEASE", "6.0.0.Final"},
			nearest:  "5.6.15.RELEASE",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "different tag on the same numbers is patch",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.RELEASE")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.RELEASE"},
			nearest:  "5.6.15.RELEASE",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "different tag is nearest in either direction",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.Final")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.RELEASE"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.Final"},
			nearest:  "5.6.15.Final",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "same numbers and different tags use the latest published_at",
			catalog:  []Package{javaAt("org.hibernate", "hibernate-core", "5.6.15.Beta", "2020-01-01T00:00:00Z"), javaAt("org.hibernate", "hibernate-core", "5.6.15.RELEASE", "2024-01-01T00:00:00Z")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.Beta", "5.6.15.RELEASE"},
			nearest:  "5.6.15.RELEASE",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "empty published_at loses among different tags",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.Beta"), javaAt("org.hibernate", "hibernate-core", "5.6.15.RELEASE", "2024-01-01T00:00:00Z")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.Beta", "5.6.15.RELEASE"},
			nearest:  "5.6.15.RELEASE",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "equal tag timestamps fall back to the version string",
			catalog:  []Package{javaAt("org.hibernate", "hibernate-core", "5.6.15.RELEASE", "2024-01-01T00:00:00Z"), javaAt("org.hibernate", "hibernate-core", "5.6.15.Final", "2024-01-01T00:00:00Z")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.GA"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.Final", "5.6.15.RELEASE"},
			nearest:  "5.6.15.Final",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "same tag line is closer than a higher number",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.16.Final"), java("org.hibernate", "hibernate-core", "5.6.15.RELEASE")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.RELEASE", "5.6.16.Final"},
			nearest:  "5.6.15.RELEASE",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "exact tag match clears nearest",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.Final"), java("org.hibernate", "hibernate-core", "5.6.15.RELEASE")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final"),
			status:   MatchStatusExact,
			versions: []string{"5.6.15.Final", "5.6.15.RELEASE"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "same numbers without a tag stay empty",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.Final")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15"),
			status:   MatchStatusPartial,
			versions: []string{"5.6.15.Final"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "dotted suffix compares as a tag",
			catalog:  []Package{py("preview", "1.2.3.post1", "")},
			pkg:      py("preview", "1.2.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3.post1"},
			nearest:  "1.2.3.post1",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "rebuild of a tagged version is exact",
			catalog:  []Package{java("org.hibernate", "hibernate-core", "5.6.15.Final")},
			pkg:      java("org.hibernate", "hibernate-core", "5.6.15.Final-rhlw-00002"),
			status:   MatchStatusExact,
			versions: []string{"5.6.15.Final"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "more than four components including a tag is patch",
			catalog:  []Package{java("org.example", "lib", "1.2.3.4.Final")},
			pkg:      java("org.example", "lib", "1.0.0"),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3.4.Final"},
			nearest:  "1.2.3.4.Final",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "missing patch component is numerically equal",
			catalog:  []Package{py("flask", "1.2.0", "")},
			pkg:      py("flask", "1.2", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.0"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "leading zeros are numerically equal",
			catalog:  []Package{py("flask", "1.2.3", "")},
			pkg:      py("flask", "1.02.3", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "snapshot suffix is not a higher numeric version",
			catalog:  []Package{py("flask", "1.2.3", ""), py("flask", "1.2.4", "")},
			pkg:      py("flask", "1.2.3-SNAPSHOT", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3", "1.2.4"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "leading v is not numeric",
			catalog:  []Package{py("flask", "1.2.3", "")},
			pkg:      py("flask", "v1.2.3", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "no user version",
			catalog:  []Package{py("flask", "1.0.0", ""), py("flask", "latest", "2020-01-01T00:00:00Z")},
			pkg:      Package{Ecosystem: EcosystemPython, Name: "flask"},
			status:   MatchStatusPartial,
			versions: []string{"1.0.0", "latest"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "rebuild of a cataloged version is exact",
			catalog:  []Package{py("flask", "1.4.2", "")},
			pkg:      py("flask", "1.4.2+rhlw.1", ""),
			status:   MatchStatusExact,
			versions: []string{"1.4.2"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "rebuild is compared by its upstream part",
			catalog:  []Package{py("flask", "1.4.2", "")},
			pkg:      py("flask", "1.4.1+rhlw.1", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.4.2"},
			nearest:  "1.4.2",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "numeric versions sort ahead of lexical order",
			catalog:  []Package{py("flask", "1.10.0", ""), py("flask", "1.2.0", ""), py("flask", "1.9.0", "")},
			pkg:      py("flask", "9.0.0", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.0", "1.9.0", "1.10.0"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "numeric higher version wins over an unparseable version",
			catalog:  []Package{py("flask", "1.2.4", ""), py("flask", "latest", "2020-01-01T00:00:00Z")},
			pkg:      py("flask", "1.2.3", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.4", "latest"},
			nearest:  "1.2.4",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "unparseable version is used when nothing numeric is higher",
			catalog:  []Package{py("flask", "1.2.0", ""), py("flask", "edge", "2021-01-01T00:00:00Z"), py("flask", "latest", "2020-01-01T00:00:00Z")},
			pkg:      py("flask", "1.2.3", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.0", "edge", "latest"},
			nearest:  "latest",
			segment:  MatchSegmentNone,
		},
		{
			name:     "earlier published_at wins among unparseable versions",
			catalog:  []Package{py("flask", "alpha", "2024-06-01T00:00:00Z"), py("flask", "gamma", "2024-01-01T00:00:00Z")},
			pkg:      py("flask", "beta", ""),
			status:   MatchStatusPartial,
			versions: []string{"alpha", "gamma"},
			nearest:  "gamma",
			segment:  MatchSegmentNone,
		},
		{
			name:     "empty published_at loses to a real timestamp",
			catalog:  []Package{py("flask", "alpha", ""), py("flask", "gamma", "2024-01-01T00:00:00Z")},
			pkg:      py("flask", "beta", ""),
			status:   MatchStatusPartial,
			versions: []string{"alpha", "gamma"},
			nearest:  "gamma",
			segment:  MatchSegmentNone,
		},
		{
			name: "duplicate upstream version keeps the earlier published_at",
			catalog: []Package{
				py("flask", "alpha", "2024-06-01T00:00:00Z"),
				py("flask", "alpha", "2024-01-01T00:00:00Z"),
				py("flask", "gamma", "2024-03-01T00:00:00Z"),
			},
			pkg:      py("flask", "beta", ""),
			status:   MatchStatusPartial,
			versions: []string{"alpha", "gamma"},
			nearest:  "alpha",
			segment:  MatchSegmentNone,
		},
		{
			name:     "equal timestamps fall back to the version string",
			catalog:  []Package{py("flask", "bbb", "2024-01-01T00:00:00Z"), py("flask", "aaa", "2024-01-01T00:00:00Z")},
			pkg:      py("flask", "beta", ""),
			status:   MatchStatusPartial,
			versions: []string{"aaa", "bbb"},
			nearest:  "aaa",
			segment:  MatchSegmentNone,
		},
		{
			name:     "more than four components is partial with patch",
			catalog:  []Package{py("flask", "1.2.3.4.5", "")},
			pkg:      py("flask", "1.2.3.4.5", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.2.3.4.5"},
			segment:  MatchSegmentPatch,
		},
		{
			name:     "more than four components on the higher version is patch",
			catalog:  []Package{py("flask", "2.0.0.0.1", "")},
			pkg:      py("flask", "1.0.0", ""),
			status:   MatchStatusPartial,
			versions: []string{"2.0.0.0.1"},
			nearest:  "2.0.0.0.1",
			segment:  MatchSegmentPatch,
		},
		{
			name:     "more than four components on an unknown package stays unmatched",
			catalog:  []Package{py("flask", "1.0.0", "")},
			pkg:      py("other", "1.2.3.4.5", ""),
			status:   MatchStatusNone,
			versions: []string{},
			segment:  MatchSegmentNone,
		},
		{
			name:     "java groups stay separate",
			catalog:  []Package{java("org.a", "lib", "1.0.0"), java("org.b", "lib", "2.0.0")},
			pkg:      java("org.a", "lib", "1.5.0"),
			status:   MatchStatusPartial,
			versions: []string{"1.0.0"},
			segment:  MatchSegmentNone,
		},
		{
			name:     "python name normalization shares the version list",
			catalog:  []Package{py("flask", "1.0.1", "")},
			pkg:      py("Flask", "1.0.0", ""),
			status:   MatchStatusPartial,
			versions: []string{"1.0.1"},
			nearest:  "1.0.1",
			segment:  MatchSegmentMinor,
		},
		{
			name:     "unknown package",
			catalog:  []Package{py("flask", "1.0.0", "")},
			pkg:      py("other", "1.0.0", ""),
			status:   MatchStatusNone,
			versions: []string{},
			segment:  MatchSegmentNone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, _ := MatchCatalog(tc.catalog, []Package{tc.pkg}, time.Time{})
			require.Len(t, results, 1)
			assert.Equal(t, tc.status, results[0].MatchStatus)
			assert.Equal(t, tc.versions, results[0].LightwellVersions)
			assert.Equal(t, tc.nearest, results[0].NearestHigherVersion)
			assert.Equal(t, tc.segment, results[0].MatchSegment)
			assert.Equal(t, tc.pkg, results[0].Package)
		})
	}
}
