package pulp_client

import (
	"context"
	"fmt"

	zest "github.com/content-services/zest/release/v2026"
)

const pythonPackageContentPageSize int32 = 500

// pythonPackageContentOrdering gives offset pages a stable order. Pulp appends -pk to any ordering.
var pythonPackageContentOrdering = []string{"pk"}

// PythonPackageContentFilters selects Python package content (one unit per distribution file).
// Set exactly one of RepositoryVersion, AddedBetween, or RemovedBetween.
type PythonPackageContentFilters struct {
	// RepositoryVersion returns the content in this repository version.
	RepositoryVersion string
	// AddedBetween is a base and target version href. It returns content in target but not in base.
	AddedBetween []string
	// RemovedBetween is a base and target version href. It returns content in base but not in target.
	RemovedBetween []string
	// NameIn matches PEP 503 normalized names. Pulp normalizes the values it is given.
	NameIn []string
	// Fields limits the response to these fields. Empty returns every field.
	Fields []string
}

func (f PythonPackageContentFilters) validate() error {
	set := 0
	if f.RepositoryVersion != "" {
		set++
	}
	if len(f.AddedBetween) > 0 {
		if len(f.AddedBetween) != 2 {
			return fmt.Errorf("added_between needs a base and a target version")
		}
		set++
	}
	if len(f.RemovedBetween) > 0 {
		if len(f.RemovedBetween) != 2 {
			return fmt.Errorf("removed_between needs a base and a target version")
		}
		set++
	}
	if set != 1 {
		return fmt.Errorf("python package content needs exactly one of repository_version, added_between, or removed_between")
	}
	return nil
}

// ListPythonPackageContent pages through Python package content and calls each once per page.
// Pages are handed off as they arrive so the caller does not have to hold every unit in memory.
func (r *pulpDaoImpl) ListPythonPackageContent(ctx context.Context, filters PythonPackageContentFilters, each func([]zest.PythonPythonPackageContentResponse) error) error {
	if err := filters.validate(); err != nil {
		return err
	}

	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return err
	}

	var offset int32
	for {
		page, err := r.listPythonPackageContentPage(ctx, client, filters, offset)
		if err != nil {
			return err
		}
		if page == nil || len(page.Results) == 0 {
			return nil
		}
		if err := each(page.Results); err != nil {
			return err
		}
		offset += int32(len(page.Results)) //nolint:gosec // G115: bounded by the page size
		if page.GetNext() == "" || int64(offset) >= int64(page.Count) {
			return nil
		}
	}
}

func (r *pulpDaoImpl) listPythonPackageContentPage(ctx context.Context, client *zest.APIClient, filters PythonPackageContentFilters, offset int32) (*zest.PaginatedpythonPythonPackageContentResponseList, error) {
	req := client.ContentPackagesAPI.ContentPythonPackagesList(ctx, r.domainName).
		Limit(pythonPackageContentPageSize).
		Offset(offset).
		Ordering(pythonPackageContentOrdering)
	if filters.RepositoryVersion != "" {
		req = req.RepositoryVersion(filters.RepositoryVersion)
	}
	if len(filters.AddedBetween) > 0 {
		req = req.AddedBetween(filters.AddedBetween)
	}
	if len(filters.RemovedBetween) > 0 {
		req = req.RemovedBetween(filters.RemovedBetween)
	}
	if len(filters.NameIn) > 0 {
		req = req.NameIn(filters.NameIn)
	}
	if len(filters.Fields) > 0 {
		req = req.Fields(filters.Fields)
	}

	resp, httpResp, err := req.Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing python package content", httpResp, err)
	}
	return resp, nil
}
