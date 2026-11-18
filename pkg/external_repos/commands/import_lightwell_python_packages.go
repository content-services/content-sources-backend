package commands

import (
	"context"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	zest "github.com/content-services/zest/release/v2026"
)

// pythonNameInBatchSize bounds the names in one name__in query so the URL stays short.
const pythonNameInBatchSize = 100

// pythonPackageKeyFields is all a diff query needs: which name and version a file belongs to.
var pythonPackageKeyFields = []string{"name", "version"}

// pythonPackageDetailFields is what the mirror stores, plus what picks the representative file.
var pythonPackageDetailFields = []string{
	"name", "version", "packagetype", "pulp_created",
	"summary", "description", "author", "author_email",
	"license", "license_expression", "project_url",
}

// pythonVersionKey is one package version. Pulp stores one content unit per distribution file,
// so several units (an sdist and wheels) share a key.
type pythonVersionKey struct {
	name    string // PEP 503 normalized
	version string
}

func pythonContentKey(row zest.PythonPythonPackageContentResponse) pythonVersionKey {
	return pythonVersionKey{name: coords.NormalizePythonName(row.GetName()), version: row.GetVersion()}
}

// pythonVersionReducer folds distribution files into one row per package version, so a full
// repository never has to be held in memory one file at a time.
// Metadata comes from one representative file: the sdist if there is one, else the newest file.
// PublishedAt is the newest pulp_created of any file in the version.
type pythonVersionReducer struct {
	best   map[pythonVersionKey]zest.PythonPythonPackageContentResponse
	newest map[pythonVersionKey]time.Time
	keep   func(pythonVersionKey) bool
}

func newPythonVersionReducer(keep func(pythonVersionKey) bool) *pythonVersionReducer {
	return &pythonVersionReducer{
		best:   map[pythonVersionKey]zest.PythonPythonPackageContentResponse{},
		newest: map[pythonVersionKey]time.Time{},
		keep:   keep,
	}
}

func (r *pythonVersionReducer) add(rows []zest.PythonPythonPackageContentResponse) error {
	for _, row := range rows {
		key := pythonContentKey(row)
		if r.keep != nil && !r.keep(key) {
			continue
		}
		created := row.GetPulpCreated()
		if created.After(r.newest[key]) {
			r.newest[key] = created
		}
		current, ok := r.best[key]
		if !ok || preferPythonDistribution(row, current) {
			r.best[key] = row
		}
	}
	return nil
}

// preferPythonDistribution reports whether candidate should replace current as the representative file.
func preferPythonDistribution(candidate, current zest.PythonPythonPackageContentResponse) bool {
	candidateSdist := candidate.GetPackagetype() == "sdist"
	currentSdist := current.GetPackagetype() == "sdist"
	if candidateSdist != currentSdist {
		return candidateSdist
	}
	return candidate.GetPulpCreated().After(current.GetPulpCreated())
}

func (r *pythonVersionReducer) has(key pythonVersionKey) bool {
	_, ok := r.best[key]
	return ok
}

// packages groups the reduced versions by package name, sorted by name and version.
func (r *pythonVersionReducer) packages() []dao.LightwellPackageInput {
	byName := make(map[string][]pythonVersionKey)
	for key := range r.best {
		byName[key.name] = append(byName[key.name], key)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]dao.LightwellPackageInput, 0, len(names))
	for _, name := range names {
		keys := byName[name]
		sort.Slice(keys, func(i, j int) bool { return keys[i].version < keys[j].version })
		versions := make([]dao.LightwellPackageVersionInput, 0, len(keys))
		for _, key := range keys {
			versions = append(versions, pythonVersionInput(key, r.best[key], r.newest[key]))
		}
		out = append(out, dao.LightwellPackageInput{Name: name, Group: "", Versions: versions})
	}
	return out
}

func pythonVersionInput(key pythonVersionKey, row zest.PythonPythonPackageContentResponse, published time.Time) dao.LightwellPackageVersionInput {
	license := row.GetLicenseExpression()
	if license == "" {
		license = row.GetLicense()
	}
	return dao.LightwellPackageVersionInput{
		Version:     key.version,
		PublishedAt: published.UTC().Format(time.RFC3339),
		Purl:        coords.BuildPURL(config.ContentTypePython, "", key.name, key.version),
		Details: &dao.LightwellPackageVersionDetailsInput{
			ProjectURL:  row.GetProjectUrl(),
			License:     license,
			Summary:     row.GetSummary(),
			Description: row.GetDescription(),
			Author:      pythonAuthor(row.GetAuthor(), row.GetAuthorEmail()),
			AuthorEmail: row.GetAuthorEmail(),
		},
	}
}

// pythonAuthor returns author, or the display name from author_email when author is empty.
// Core metadata often puts both in author_email as "Name <address>".
func pythonAuthor(author, authorEmail string) string {
	if strings.TrimSpace(author) != "" {
		return author
	}
	authorEmail = strings.TrimSpace(authorEmail)
	if authorEmail == "" {
		return author
	}
	addr, err := mail.ParseAddress(authorEmail)
	if err != nil || addr.Name == "" {
		return author
	}
	return addr.Name
}

// fetchPythonPackages reads every package version in a repository version from the Pulp content API.
func fetchPythonPackages(ctx context.Context, pulpClient pulp_client.PulpClient, versionHref string) ([]dao.LightwellPackageInput, error) {
	reducer := newPythonVersionReducer(nil)
	err := pulpClient.ListPythonPackageContent(ctx, pulp_client.PythonPackageContentFilters{
		RepositoryVersion: versionHref,
		Fields:            pythonPackageDetailFields,
	}, reducer.add)
	if err != nil {
		return nil, err
	}
	return reducer.packages(), nil
}

// fetchPythonPackageChanges reads what changed between two versions of a repository.
// added_between and removed_between name the files that changed. A version is affected when any of
// its files changed, because its metadata and published time depend on every file it has. Each
// affected version is read again from the target version: if it still has files it is returned as
// an upsert, and if it has none it is returned as a removal.
func fetchPythonPackageChanges(ctx context.Context, pulpClient pulp_client.PulpClient, baseVersion, targetVersion string) ([]dao.LightwellPackageInput, []dao.LightwellPackageVersionKey, error) {
	affected := map[pythonVersionKey]struct{}{}
	collect := func(rows []zest.PythonPythonPackageContentResponse) error {
		for _, row := range rows {
			affected[pythonContentKey(row)] = struct{}{}
		}
		return nil
	}
	between := []string{baseVersion, targetVersion}
	if err := pulpClient.ListPythonPackageContent(ctx, pulp_client.PythonPackageContentFilters{
		AddedBetween: between,
		Fields:       pythonPackageKeyFields,
	}, collect); err != nil {
		return nil, nil, err
	}
	if err := pulpClient.ListPythonPackageContent(ctx, pulp_client.PythonPackageContentFilters{
		RemovedBetween: between,
		Fields:         pythonPackageKeyFields,
	}, collect); err != nil {
		return nil, nil, err
	}
	if len(affected) == 0 {
		return nil, nil, nil
	}

	nameSet := map[string]struct{}{}
	for key := range affected {
		nameSet[key.name] = struct{}{}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)

	reducer := newPythonVersionReducer(func(key pythonVersionKey) bool {
		_, ok := affected[key]
		return ok
	})
	for start := 0; start < len(names); start += pythonNameInBatchSize {
		batch := names[start:min(start+pythonNameInBatchSize, len(names))]
		if err := pulpClient.ListPythonPackageContent(ctx, pulp_client.PythonPackageContentFilters{
			RepositoryVersion: targetVersion,
			NameIn:            batch,
			Fields:            pythonPackageDetailFields,
		}, reducer.add); err != nil {
			return nil, nil, err
		}
	}

	removals := make([]dao.LightwellPackageVersionKey, 0)
	for key := range affected {
		if !reducer.has(key) {
			removals = append(removals, dao.LightwellPackageVersionKey{Group: "", Name: key.name, Version: key.version})
		}
	}
	sort.Slice(removals, func(i, j int) bool {
		if removals[i].Name != removals[j].Name {
			return removals[i].Name < removals[j].Name
		}
		return removals[i].Version < removals[j].Version
	})
	return reducer.packages(), removals, nil
}
