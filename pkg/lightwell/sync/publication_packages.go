package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
)

const (
	fixedVersionMarker = "lightwell fixed versions"
	purlTypeMaven      = "maven"
	purlTypePyPI       = "pypi"
)

var fixedVersionPURLPattern = regexp.MustCompile("pkg:[^\\s()<>\\[\\]{}\"'`|]+")

// PublicationPackage identifies a package independently of its version.
type PublicationPackage struct {
	Ecosystem string
	Namespace string
	Name      string
}

type publicationPackageVersion struct {
	Package PublicationPackage
	Version string
}

// PublicationPackageVerifier confirms which requested package versions exist in
// Lightwell publication repositories. Implementations return only confirmed
// versions. Warnings describe checks that could not be completed.
type PublicationPackageVerifier interface {
	VerifyPublished(
		ctx context.Context,
		packages map[PublicationPackage][]string,
	) (map[PublicationPackage][]string, []string)
}

func fixedVersionPackages(raw json.RawMessage) []publicationPackageVersion {
	description := flattenADF(raw)
	marker := strings.Index(strings.ToLower(description), fixedVersionMarker)
	if marker < 0 {
		return nil
	}
	description = description[marker+len(fixedVersionMarker):]

	seen := make(map[string]struct{})
	packages := make([]publicationPackageVersion, 0)
	for _, candidate := range fixedVersionPURLPattern.FindAllString(description, -1) {
		candidate = strings.Trim(candidate, "`*_.,;:!?[]{}<>\"'")
		parsed := utils.ParsePURL(candidate)
		if parsed == nil || parsed.Version == "" {
			continue
		}
		if parsed.Type != purlTypeMaven && parsed.Type != purlTypePyPI {
			continue
		}
		version := strings.ToLower(parsed.Version)
		if !strings.Contains(version, ".rhlw-") && !strings.Contains(version, "-rhlw-") {
			continue
		}
		normalized := parsed.String()
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		packages = append(packages, publicationPackageVersion{
			Package: PublicationPackage{
				Ecosystem: strings.ToLower(parsed.Type),
				Namespace: parsed.Namespace,
				Name:      parsed.Name,
			},
			Version: parsed.Version,
		})
	}
	return packages
}

func collectBreadcrumbPackages(issues []jira_client.JiraIssue) (map[PublicationPackage][]string, map[string][]publicationPackageVersion) {
	requested := make(map[publicationPackageVersion]struct{})
	byIssue := make(map[string][]publicationPackageVersion)
	for _, issue := range issues {
		if discardedResolution(issue.Fields["resolution"]) || stage(issue.Fields["status"]) != "Validation" {
			continue
		}
		for _, item := range fixedVersionPackages(issue.Fields["description"]) {
			requested[item] = struct{}{}
			byIssue[issue.Key] = append(byIssue[issue.Key], item)
		}
	}

	packages := make(map[PublicationPackage][]string, len(requested))
	for item := range requested {
		packages[item.Package] = append(packages[item.Package], item.Version)
	}
	for pkg := range packages {
		slices.Sort(packages[pkg])
	}
	return packages, byIssue
}

func confirmedVersionsByIssue(
	byIssue map[string][]publicationPackageVersion,
	confirmed map[PublicationPackage][]string,
) map[string][]string {
	confirmedVersions := make(map[publicationPackageVersion]struct{})
	for pkg, versions := range confirmed {
		for _, version := range versions {
			confirmedVersions[publicationPackageVersion{Package: pkg, Version: version}] = struct{}{}
		}
	}

	result := make(map[string][]string)
	for issueKey, requested := range byIssue {
		seen := make(map[string]struct{})
		for _, item := range requested {
			if _, found := confirmedVersions[item]; !found {
				continue
			}
			if _, duplicate := seen[item.Version]; duplicate {
				continue
			}
			seen[item.Version] = struct{}{}
			result[issueKey] = append(result[issueKey], item.Version)
		}
		slices.Sort(result[issueKey])
		slices.Reverse(result[issueKey])
	}
	return result
}

type publicationPackageVerifier struct {
	repositories dao.RepositoryConfigDao
	pulp         pulp_client.PulpClient
	tang         tangy.Tangy
}

// NewPublicationPackageVerifier builds a verifier backed by Lightwell's Pulp
// repositories and Tang package queries.
func NewPublicationPackageVerifier(
	repositories dao.RepositoryConfigDao,
	pulp pulp_client.PulpClient,
	tang tangy.Tangy,
) PublicationPackageVerifier {
	return &publicationPackageVerifier{repositories: repositories, pulp: pulp, tang: tang}
}

type publicationRepository struct {
	basePath    string
	contentType string
	href        string
}

func (v *publicationPackageVerifier) VerifyPublished(
	ctx context.Context,
	packages map[PublicationPackage][]string,
) (map[PublicationPackage][]string, []string) {
	confirmed := make(map[PublicationPackage][]string)
	repositories, warnings := v.publicationRepositories(ctx)
	if len(repositories) == 0 {
		if len(warnings) == 0 {
			warnings = append(warnings, "cannot verify publication packages: no publication repositories found")
		}
		return confirmed, warnings
	}

	for pkg, versions := range packages {
		contentType := publicationContentType(pkg.Ecosystem)
		if contentType == "" {
			warnings = append(warnings, fmt.Sprintf("cannot verify publication package %s: unsupported ecosystem %q", packageName(pkg), pkg.Ecosystem))
			continue
		}
		matched := false
		for _, repo := range repositories {
			if repo.contentType != contentType {
				continue
			}
			matched = true
			available, err := v.packageVersions(ctx, repo.href, pkg)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"cannot verify publication package %s in repository %s: %v",
					packageName(pkg), repo.basePath, err,
				))
				continue
			}
			for _, version := range versions {
				if available[version] {
					confirmed[pkg] = append(confirmed[pkg], version)
				}
			}
		}
		if !matched {
			warnings = append(warnings, fmt.Sprintf("cannot verify publication package %s: no %s publication repository", packageName(pkg), contentType))
		}
	}

	for pkg, versions := range confirmed {
		slices.Sort(versions)
		versions = slices.Compact(versions)
		slices.Reverse(versions)
		confirmed[pkg] = versions
	}
	return confirmed, warnings
}

func (v *publicationPackageVerifier) publicationRepositories(ctx context.Context) ([]publicationRepository, []string) {
	configured, err := v.repositories.InternalOnly_FetchRepoConfigForOrg(ctx, config.LightwellOrg)
	if err != nil {
		return nil, []string{fmt.Sprintf("cannot list Lightwell publication repositories: %v", err)}
	}

	repositories := make([]publicationRepository, 0, len(configured))
	warnings := make([]string, 0)
	for _, repo := range configured {
		basePath := strings.TrimRight(strings.TrimSpace(repo.PublishedDistBasePath), "/")
		if !strings.HasSuffix(basePath, "/remediated") && !strings.HasSuffix(basePath, "/predisclosure") {
			continue
		}
		href, err := v.pulp.ResolveRepositoryFromBasePath(ctx, basePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot resolve Lightwell publication repository %s: %v", basePath, err))
			continue
		}
		if href == nil || *href == "" {
			warnings = append(warnings, fmt.Sprintf("cannot resolve Lightwell publication repository %s: repository not found", basePath))
			continue
		}
		repositories = append(repositories, publicationRepository{
			basePath: basePath, contentType: repo.ContentType, href: *href,
		})
	}
	return repositories, warnings
}

func (v *publicationPackageVerifier) packageVersions(
	ctx context.Context,
	repositoryHref string,
	pkg PublicationPackage,
) (map[string]bool, error) {
	available := make(map[string]bool)
	switch pkg.Ecosystem {
	case purlTypeMaven:
		return v.mavenPackageVersions(ctx, repositoryHref, pkg)
	case purlTypePyPI:
		versions, err := v.tang.PythonPackageVersionsGet(ctx, repositoryHref, normalizePythonPackageName(pkg.Name))
		if errors.Is(err, tangy.ErrPythonPackageNotFound) {
			return available, nil
		}
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			available[version.Version] = true
		}
	}
	return available, nil
}

func (v *publicationPackageVerifier) mavenPackageVersions(
	ctx context.Context,
	repositoryHref string,
	pkg PublicationPackage,
) (map[string]bool, error) {
	if pkg.Namespace == "" {
		return nil, errors.New("maven package has no group")
	}

	available := make(map[string]bool)
	offset := 0
	for {
		response, err := v.tang.MavenVersionsList(
			ctx, repositoryHref, pkg.Namespace, pkg.Name, "", tangy.PageOptions{Offset: offset, Limit: 1000},
		)
		if err != nil {
			return nil, err
		}
		for _, item := range response.Results {
			addMavenItemVersions(available, item)
		}
		offset += len(response.Results)
		if len(response.Results) == 0 || offset >= response.Total {
			return available, nil
		}
	}
}

func addMavenItemVersions(available map[string]bool, item tangy.MavenVersionsItem) {
	available[item.Version] = true
	for _, build := range item.Builds {
		available[build.Version] = true
		if build.Release != "" {
			available[build.Version+"."+build.Release] = true
			available[item.Version+"."+build.Release] = true
		}
	}
}

func publicationContentType(ecosystem string) string {
	switch ecosystem {
	case purlTypeMaven:
		return config.ContentTypeMaven
	case purlTypePyPI:
		return config.ContentTypePython
	default:
		return ""
	}
}

func packageName(pkg PublicationPackage) string {
	if pkg.Namespace == "" {
		return pkg.Name
	}
	return pkg.Namespace + "/" + pkg.Name
}

func normalizePythonPackageName(name string) string {
	var normalized strings.Builder
	normalized.Grow(len(name))
	separatorWritten := false
	for _, character := range strings.ToLower(name) {
		if character == '-' || character == '_' || character == '.' {
			if !separatorWritten {
				normalized.WriteByte('-')
				separatorWritten = true
			}
			continue
		}
		normalized.WriteRune(character)
		separatorWritten = false
	}
	return normalized.String()
}
