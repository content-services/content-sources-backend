package dao

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

type BeaconPackageKey struct {
	Ecosystem        string
	ComponentName    string
	ComponentVersion string
}

func (d lightwellVulnerabilityDaoImpl) ListBeaconPackages(ctx context.Context, opts ListLightwellVulnerabilitiesOptions, allAddressed bool) (api.LightwellBeaconPackageCollection, int64, error) {
	var resp api.LightwellBeaconPackageCollection
	if d.querier == nil {
		return resp, 0, errors.New("lightwell querier is not initialized")
	}
	result, err := d.querier.ListBeaconPackages(ctx, store.ListBeaconPackagesParams{
		CustomerID: opts.CustomerID, Severities: opts.Severities, Stages: opts.Stages,
		Complexities: opts.Complexities, TicketIds: opts.LtwlsuptTicketIDs, Flags: opts.Flags,
		Search: opts.Search, AllAddressed: allAddressed, SeverityRank: lightwellSeverityRank,
		PageLimit: opts.Limit, PageOffset: opts.Offset,
	})
	if err != nil {
		return resp, 0, err
	}
	var packages []struct {
		api.LightwellBeaconPackage
		PublishedVersions []string `json:"published_versions"`
	}
	if err := json.Unmarshal(result.Packages, &packages); err != nil {
		return resp, 0, err
	}
	summary := api.LightwellBeaconVulnerabilitySummary{
		Count: result.VulnerabilityCount, CriticalCount: result.CriticalCount, EmbargoCount: result.EmbargoCount,
	}
	if err := json.Unmarshal(result.StatusCounts, &summary.StatusCounts); err != nil {
		return resp, 0, err
	}
	resp.Meta.VulnerabilitySummary = summary

	// Match Beacon's case- and accent-insensitive numeric comparison, using
	// a fixed locale and the query's stable order to break collation ties.
	versions := collate.New(language.Und, collate.Loose, collate.Numeric)
	resp.Data = make([]api.LightwellBeaconPackage, 0, len(packages))
	for _, pkg := range packages {
		pkg.HighestPublishedVersion = highestBeaconPublishedVersion(versions, pkg.PublishedVersions)
		resp.Data = append(resp.Data, pkg.LightwellBeaconPackage)
	}
	return resp, result.PackageCount, nil
}

func (d lightwellVulnerabilityDaoImpl) ListBeaconPackageVulnerabilities(ctx context.Context, opts ListLightwellVulnerabilitiesOptions, key BeaconPackageKey) (api.LightwellBeaconPackageVulnerabilityCollection, int64, error) {
	var resp api.LightwellBeaconPackageVulnerabilityCollection
	if d.querier == nil {
		return resp, 0, errors.New("lightwell querier is not initialized")
	}
	result, err := d.querier.ListBeaconPackageVulnerabilities(ctx, store.ListBeaconPackageVulnerabilitiesParams{
		CustomerID: opts.CustomerID, SeverityRank: lightwellSeverityRank, Ecosystem: key.Ecosystem, ComponentName: key.ComponentName, ComponentVersion: key.ComponentVersion,
		PageLimit: opts.Limit, PageOffset: opts.Offset,
	})
	if err != nil {
		return resp, 0, err
	}
	if err := json.Unmarshal(result.Vulnerabilities, &resp.Data); err != nil {
		return resp, 0, err
	}
	if err := json.Unmarshal(result.StatusCounts, &resp.Meta.StatusCounts); err != nil {
		return resp, 0, err
	}
	resp.Meta.VulnerabilityCount = result.VulnerabilityCount
	if result.HighestSeverity != "" {
		resp.Meta.HighestSeverity = &result.HighestSeverity
	}
	resp.Meta.HighestPublishedVersion = highestBeaconPublishedVersion(
		collate.New(language.Und, collate.Loose, collate.Numeric), result.PublishedVersions,
	)
	for i := range resp.Data {
		resp.Data[i].ResolutionReason = customerResolutionReason(resp.Data[i].ResolutionReason)
	}
	return resp, result.RecordCount, nil
}

func highestBeaconPublishedVersion(versions *collate.Collator, published []string) *string {
	var highest *string
	for _, version := range published {
		if highest == nil || versions.CompareString(version, *highest) > 0 {
			highest = &version
		}
	}
	return highest
}
