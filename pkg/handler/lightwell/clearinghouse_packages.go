package lightwell

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/content-services/content-sources-backend/pkg/coverage/parser"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

// clearinghousePackages returns the unique packages referenced by the
// vulnerabilities, in the order they first appear, and a per-ecosystem summary.
// Packageurls are parsed and deduplicated the same way as Lens coverage
// reports (by ecosystem, namespace, name and version). A packageurl is valid
// only when it parses and has a version; unlike Lens, a purl without a version
// is kept in its ecosystem but marked invalid. Packageurls that do not parse
// are kept under the Unknown ecosystem and deduplicated by their trimmed value.
func clearinghousePackages(vulns []intakeVulnerability) ([]dao.CreateClearinghouseUploadPackageParams, api.ClearinghousePackageSummary) {
	packages := make([]dao.CreateClearinghouseUploadPackageParams, 0, len(vulns))
	seen := make(map[string]struct{}, len(vulns))
	totals := map[string]int{}
	summary := api.ClearinghousePackageSummary{}

	for _, v := range vulns {
		raw := strings.TrimSpace(v.Packageurl)
		pkg := dao.CreateClearinghouseUploadPackageParams{Packageurl: raw, Ecosystem: api.ClearinghouseEcosystemUnknown}
		key := "unparsed|" + raw
		if parsed, err := parser.ParsePURL(raw); err != nil {
			pkg.InvalidReason = api.ClearinghouseInvalidReasonNotAPurl
			pkg.InvalidDetail = err.Error()
		} else {
			pkg.Ecosystem = parsed.Ecosystem
			pkg.Namespace = parsed.Namespace
			pkg.Name = parsed.Name
			pkg.Version = parsed.Version
			pkg.ValidPurl = parsed.Version != ""
			if !pkg.ValidPurl {
				pkg.InvalidReason = api.ClearinghouseInvalidReasonMissingVersion
				pkg.InvalidDetail = "packageurl has no version"
			}
			key = "purl|" + parsed.Ecosystem + "|" + parsed.Namespace + "|" + parsed.Name + "|" + parsed.Version
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		packages = append(packages, pkg)

		totals[pkg.Ecosystem]++
		summary.Total++
		if !pkg.ValidPurl {
			summary.InvalidPurls++
		}
	}

	summary.Ecosystems = make([]api.ClearinghousePackageEcosystemSummary, 0, len(totals))
	for ecosystem, total := range totals {
		summary.Ecosystems = append(summary.Ecosystems, api.ClearinghousePackageEcosystemSummary{Ecosystem: ecosystem, Total: total})
	}
	sort.Slice(summary.Ecosystems, func(i, j int) bool {
		a, b := summary.Ecosystems[i], summary.Ecosystems[j]
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		return a.Ecosystem < b.Ecosystem
	})
	return packages, summary
}

// clearinghousePackageFilters reads the ecosystem and valid_purl query filters.
func clearinghousePackageFilters(c echo.Context) (dao.ListClearinghouseUploadPackagesParams, error) {
	params := dao.ListClearinghouseUploadPackagesParams{}
	for _, ecosystem := range strings.Split(c.QueryParam("ecosystem"), ",") {
		if ecosystem = strings.TrimSpace(ecosystem); ecosystem != "" {
			params.Ecosystems = append(params.Ecosystems, ecosystem)
		}
	}
	if raw := c.QueryParam("valid_purl"); raw != "" {
		valid, err := strconv.ParseBool(raw)
		if err != nil {
			return params, ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", "valid_purl must be true or false")
		}
		params.ValidPurl = &valid
	}
	return params, nil
}

// clearinghouseSubmissionPackage converts a stored package into the API
// response. Name includes the namespace, formatted the same way as Lens.
func clearinghouseSubmissionPackage(pkg dao.ClearinghouseUploadPackage) api.ClearinghouseSubmissionPackage {
	return api.ClearinghouseSubmissionPackage{
		Packageurl: pkg.Packageurl,
		Ecosystem:  pkg.Ecosystem,
		Name:       matcher.Package{Ecosystem: pkg.Ecosystem, Namespace: pkg.Namespace, Name: pkg.Name}.FullName(),
		Version:    pkg.Version,
		ValidPurl:  pkg.ValidPurl,
		// Older rows and valid packages have no reason, so the fields are omitted.
		InvalidReason: optionalString(pkg.InvalidReason),
		InvalidDetail: optionalString(pkg.InvalidDetail),
	}
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
