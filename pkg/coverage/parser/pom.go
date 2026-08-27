package parser

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	gitpom "github.com/git-pkgs/pom"
)

const pomResolveTimeout = 2 * time.Minute

// pomFetcher loads parent POMs and imported BOMs. Tests replace it with an offline stub.
var pomFetcher gitpom.Fetcher = newHTTPPOMFetcher()

func newHTTPPOMFetcher() gitpom.Fetcher {
	f := gitpom.NewHTTPFetcher("")
	f.Client = &http.Client{Timeout: 30 * time.Second}
	return gitpom.NewCachingFetcher(f)
}

// parsePOM uses git-pkgs/pom to parse the file, then resolve parents and BOM imports.
func parsePOM(r io.Reader) ([]Package, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	parsed, err := gitpom.ParsePOM(data)
	if err != nil {
		return nil, wrapParse("POM", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pomResolveTimeout)
	defer cancel()

	ep, err := gitpom.NewResolver(pomFetcher).ResolvePOM(ctx, parsed, gitpom.Options{})
	if err != nil {
		return nil, wrapParse("POM", err)
	}
	return packagesFromEffective(ep), nil
}

func packagesFromEffective(ep *gitpom.EffectivePOM) []Package {
	var pkgs []Package
	for _, d := range ep.Dependencies {
		if strings.EqualFold(d.Scope, "import") {
			continue
		}
		if d.GroupID == "" || d.ArtifactID == "" {
			continue
		}
		if strings.Contains(d.GroupID, "${") || strings.Contains(d.ArtifactID, "${") {
			continue
		}
		ver := d.Version
		if d.Resolution != gitpom.Resolved {
			ver = ""
		}
		pkgs = append(pkgs, Package{
			Ecosystem: EcosystemJava,
			Namespace: d.GroupID,
			Name:      d.ArtifactID,
			Version:   ver,
		})
	}
	return pkgs
}
