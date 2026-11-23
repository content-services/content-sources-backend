package pulp_client

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"

	zest "github.com/content-services/zest/release/v2026"
)

const pythonPackagePageSize int32 = 100

var pythonPackageNameSeparators = regexp.MustCompile(`[-_.]+`)

// ListPythonPackageFiles returns the complete distribution inventory for one
// exact Python release, ordered by filename. The caller must authorize the
// repository and supply its distribution base path and a normalized package
// name. Version is used literally, including any Lightwell build suffix.
//
// The distribution's repository version is resolved once for all pages. Empty
// inventory is a successful lookup, while any mapping, page, or record failure
// returns no files. Content/artifact hrefs are retained for internal download
// resolution; callers must map these records to the public response type.
func (r *pulpDaoImpl) ListPythonPackageFiles(ctx context.Context, basePath, name, version string) ([]zest.PythonPythonPackageContentResponse, error) {
	if basePath == "" || name == "" || version == "" {
		return nil, fmt.Errorf("python inventory requires a distribution base path, normalized name, and exact version")
	}

	versionHref, err := r.pythonDistributionVersion(ctx, basePath)
	if err != nil {
		return nil, err
	}
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

	files := []zest.PythonPythonPackageContentResponse{}
	byContent := make(map[string]zest.PythonPythonPackageContentResponse)
	byFilename := make(map[string]string)
	var offset int32
	var total int32
	for {
		page, err := r.pythonPackagePage(ctx, client, versionHref, name, version, offset)
		if err != nil {
			return nil, err
		}
		if offset == 0 {
			total = page.Count
		}
		end := int64(offset) + int64(len(page.Results))
		if end < 0 || end > math.MaxInt32 || page.Count < 0 || page.Count != total || end > int64(total) {
			return nil, fmt.Errorf("inconsistent Python inventory pagination")
		}
		if len(page.Results) == 0 && offset < total {
			return nil, fmt.Errorf("incomplete Python inventory pagination")
		}
		previousCount := len(files)
		for _, file := range page.Results {
			if normalizePythonPackageName(file.GetName()) != name || file.GetVersion() != version {
				return nil, fmt.Errorf("python inventory record does not match the requested release")
			}
			if err := validatePythonPackageFile(&file); err != nil {
				return nil, err
			}
			href, filename := file.GetPulpHref(), file.GetFilename()
			if previous, exists := byContent[href]; exists {
				if !reflect.DeepEqual(previous, file) {
					return nil, fmt.Errorf("conflicting Python inventory content records")
				}
				continue
			}
			if _, exists := byFilename[filename]; exists {
				return nil, fmt.Errorf("conflicting Python inventory filenames")
			}
			byContent[href] = file
			byFilename[filename] = href
			files = append(files, file)
		}
		if len(page.Results) > 0 && len(files) == previousCount {
			return nil, fmt.Errorf("python inventory pagination made no progress")
		}
		if end == int64(total) {
			break
		}
		// Advance by the actual page length because Pulp may cap the limit. Never
		// follow next URLs or carry their filters into the configured client.
		offset = int32(end)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].GetFilename() < files[j].GetFilename() })
	return files, nil
}

func (r *pulpDaoImpl) pythonDistributionVersion(ctx context.Context, basePath string) (string, error) {
	distribution, err := r.FindGenericDistributionByBasePath(ctx, basePath)
	if err != nil {
		return "", err
	}
	if distribution == nil {
		return "", errDistributionNotFound
	}
	if href := distribution.GetRepositoryVersion(); href != "" {
		return href, nil
	}
	repoHref := distribution.GetRepository()
	if repoHref == "" {
		return "", fmt.Errorf("python distribution has no repository mapping")
	}
	href, err := r.GetLatestVersionHref(ctx, repoHref)
	if err != nil {
		return "", err
	}
	if href == nil || *href == "" {
		return "", fmt.Errorf("python distribution repository has no latest version")
	}
	return *href, nil
}

func (r *pulpDaoImpl) pythonPackagePage(ctx context.Context, client *zest.APIClient, versionHref, name, version string, offset int32) (*zest.PaginatedpythonPythonPackageContentResponseList, error) {
	page, httpResp, err := client.ContentPackagesAPI.ContentPythonPackagesList(ctx, r.domainName).
		RepositoryVersion(versionHref).
		Name(name).
		Version(version).
		Ordering([]string{"filename"}).
		Fields([]string{"pulp_href", "artifact", "name", "version", "filename", "packagetype", "size", "sha256"}).
		Limit(pythonPackagePageSize).
		Offset(offset).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("error listing Python package files: %w", err)
	}
	if page == nil || page.Results == nil {
		return nil, fmt.Errorf("empty or malformed Python inventory response")
	}
	return page, nil
}

// normalizePythonPackageName matches Pulp's PEP 503 name filtering while leaving
// the original metadata name intact in the returned content record.
func normalizePythonPackageName(name string) string {
	return pythonPackageNameSeparators.ReplaceAllString(strings.ToLower(name), "-")
}

func validatePythonPackageFile(file *zest.PythonPythonPackageContentResponse) error {
	if file.GetPulpHref() == "" || file.GetFilename() == "" {
		return fmt.Errorf("python inventory record is missing content identity or filename")
	}
	if file.Size != nil && *file.Size < 0 {
		return fmt.Errorf("python inventory record has a negative size")
	}
	if file.GetSha256() == "" {
		file.Sha256 = nil
	} else {
		digest, err := hex.DecodeString(file.GetSha256())
		if err != nil || len(digest) != 32 {
			return fmt.Errorf("python inventory record has an invalid SHA-256 digest")
		}
		file.Sha256 = zest.PtrString(strings.ToLower(file.GetSha256()))
	}
	if file.GetPackagetype() == "" {
		file.Packagetype = nil
	}
	return nil
}
