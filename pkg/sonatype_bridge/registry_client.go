package sonatype_bridge

// TODO: This file mixes four concerns that should be split into distinct files:
//  1. HTTP transport + retry (httpRegistryClient, getWithRetry, doGet, and the Fetch methods)
//  2. OSV parsing + CVE extraction (osvFile, parseOSVFile, OSVRecord)
//  3. CVSS 3.1 scoring (computeCVSS31BaseScore, parseCVSSVector, cvssMetricValue, cvssScoreToSeverity)
//  4. Maven path utility (groupPath)
//
// Concerns 2 and 3 are pure functions with no HTTP dependency and could move to
// a dedicated file or into the shared vulnerability_parser package.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/external_repos/vulnerability_parser"
	"github.com/rs/zerolog/log"
)

const (
	maxRegistryResponseBytes = 200 << 20 // 200 MB
)

// OSVRecord holds the bridge-internal representation of a single CVE
// extracted from an OSV JSON file. Embargo validation reads CVEID.
type OSVRecord struct {
	CVEID       string
	Aliases     []string
	Description string
	CVSSVector  string
	CVSSScore   float64
	Severity    string
}

// RegistryClient abstracts HTTP calls to the Lightwell registry on
// packages.redhat.com to fetch JARs, POMs, SBOMs, VEX documents, and OSV records.
type RegistryClient interface {
	FetchJAR(ctx context.Context, groupID, artifactID, version string) (body []byte, sha256hex string, err error)
	FetchPOM(ctx context.Context, groupID, artifactID, version string) ([]byte, error)
	// FetchClassifier fetches an optional classified JAR (e.g. "sources",
	// "javadoc"). It returns (nil, nil) when the registry reports 404.
	FetchClassifier(ctx context.Context, groupID, artifactID, version, classifier string) ([]byte, error)
	FetchSBOM(ctx context.Context, groupID, artifactID, version string) (bom, asc []byte, err error)
	FetchOSVRecords(ctx context.Context, groupID, artifactID, baseVersion string) ([]OSVRecord, error)
	FetchVEX(ctx context.Context, cveIDs []string) ([]byte, error)
	Ping(ctx context.Context) error
}

type httpRegistryClient struct {
	httpClient  *http.Client
	registryURL string
	osvURL      string
	vexURL      string
	username    string
	password    string
	maxRetries  int
}

func newRegistryClient(cfg bridgeConfig) RegistryClient {
	return &httpRegistryClient{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
		registryURL: strings.TrimSuffix(cfg.RegistryURL, "/"),
		osvURL:      strings.TrimSuffix(cfg.RegistryOSVURL, "/"),
		vexURL:      strings.TrimSuffix(cfg.RegistryVEXURL, "/"),
		username:    cfg.RegistryUsername,
		password:    cfg.RegistryPassword,
		maxRetries:  cfg.MaxRetries,
	}
}

func (c *httpRegistryClient) FetchJAR(ctx context.Context, groupID, artifactID, version string) ([]byte, string, error) {
	path := fmt.Sprintf("%s/%s/%s/%s/%s-%s.jar",
		c.registryURL, groupPath(groupID), artifactID, version, artifactID, version)
	data, err := c.getWithRetry(ctx, path)
	if err != nil {
		return nil, "", fmt.Errorf("fetch JAR: %w", err)
	}
	h := sha256.Sum256(data)
	return data, fmt.Sprintf("%x", h), nil
}

func (c *httpRegistryClient) FetchPOM(ctx context.Context, groupID, artifactID, version string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s/%s/%s/%s-%s.pom",
		c.registryURL, groupPath(groupID), artifactID, version, artifactID, version)
	data, err := c.getWithRetry(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch POM: %w", err)
	}
	return data, nil
}

func (c *httpRegistryClient) FetchClassifier(ctx context.Context, groupID, artifactID, version, classifier string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s/%s/%s/%s-%s-%s.jar",
		c.registryURL, groupPath(groupID), artifactID, version, artifactID, version, classifier)
	data, err := c.getOptionalWithRetry(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("fetch %s JAR: %w", classifier, err)
	}
	return data, nil
}

func (c *httpRegistryClient) FetchSBOM(ctx context.Context, groupID, artifactID, version string) ([]byte, []byte, error) {
	bomURL := fmt.Sprintf("%s/%s/%s/%s/%s-%s.bom.json",
		c.registryURL, groupPath(groupID), artifactID, version, artifactID, version)
	bom, err := c.getWithRetry(ctx, bomURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch SBOM: %w", err)
	}
	asc, err := c.getWithRetry(ctx, bomURL+".asc")
	if err != nil {
		return nil, nil, fmt.Errorf("fetch SBOM signature: %w", err)
	}
	return bom, asc, nil
}

func (c *httpRegistryClient) FetchOSVRecords(ctx context.Context, groupID, artifactID, baseVersion string) ([]OSVRecord, error) {
	manifestURL := c.osvURL + "/PULP_MANIFEST"
	manifestData, err := c.getWithRetry(ctx, manifestURL)
	if err != nil {
		return nil, fmt.Errorf("fetch PULP_MANIFEST: %w", err)
	}

	entries := vulnerability_parser.ParseManifest(manifestData)
	var records []OSVRecord
	var selected, failed int
	for _, entry := range entries {
		if !isRootRHLWFilename(entry.Filename) {
			continue
		}
		selected++
		fileURL := c.osvURL + "/" + entry.Filename
		data, err := c.getWithRetry(ctx, fileURL)
		if err != nil {
			failed++
			log.Warn().Err(err).Str("file", entry.Filename).Msg("skipping OSV file")
			continue
		}
		recs, matched, err := parseOSVRecordsForPackage(data, groupID, artifactID, baseVersion)
		if err != nil {
			failed++
			log.Warn().Err(err).Str("file", entry.Filename).Msg("skipping malformed OSV")
			continue
		}
		if !matched {
			continue
		}
		records = append(records, recs...)
	}
	if len(records) == 0 && selected > 0 && failed == selected {
		return nil, fmt.Errorf("all %d matching OSV files failed to fetch for version %s", selected, baseVersion)
	}
	return records, nil
}

func isRootRHLWFilename(filename string) bool {
	name := strings.TrimPrefix(filename, "./")
	return !strings.Contains(name, "/") && strings.HasPrefix(name, "RHLW-") && strings.HasSuffix(name, ".json")
}

func (c *httpRegistryClient) FetchVEX(ctx context.Context, cveIDs []string) ([]byte, error) {
	if len(cveIDs) == 0 {
		return nil, fmt.Errorf("fetch VEX: no CVE IDs")
	}
	manifestData, err := c.getWithRetry(ctx, c.vexURL+"/PULP_MANIFEST")
	if err != nil {
		return nil, fmt.Errorf("fetch VEX: %w", err)
	}
	filename, err := multiCVEVEXFilename(vulnerability_parser.ParseManifest(manifestData), cveIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch VEX: %w", err)
	}
	// PathEscape leaves '+' alone. Encode it so the request target cannot
	// be read as a space by a query-style decoder.
	segment := strings.ReplaceAll(url.PathEscape(filename), "+", "%2B")
	vexURL, err := url.JoinPath(c.vexURL, segment)
	if err != nil {
		return nil, fmt.Errorf("fetch VEX %s: %w", filename, err)
	}
	data, err := c.getWithRetry(ctx, vexURL)
	if err != nil {
		return nil, fmt.Errorf("fetch VEX %s: %w", filename, err)
	}
	return data, nil
}

func (c *httpRegistryClient) Ping(ctx context.Context) error {
	url := c.osvURL + "/PULP_MANIFEST"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("registry ping: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *httpRegistryClient) getWithRetry(ctx context.Context, rawURL string) ([]byte, error) {
	var data []byte
	err := retryWithBackoff(ctx, c.maxRetries, func() error {
		var fetchErr error
		data, fetchErr = c.doGet(ctx, rawURL)
		return fetchErr
	})
	return data, err
}

// getOptionalWithRetry is like getWithRetry but treats HTTP 404 as "absent":
// it returns (nil, nil) immediately without retrying.
func (c *httpRegistryClient) getOptionalWithRetry(ctx context.Context, rawURL string) ([]byte, error) {
	var data []byte
	err := retryWithBackoff(ctx, c.maxRetries, func() error {
		var fetchErr error
		data, fetchErr = c.doGet(ctx, rawURL)
		var statusErr *httpStatusError
		if errors.As(fetchErr, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			data = nil
			return nil
		}
		return fetchErr
	})
	return data, err
}

type httpStatusError struct {
	StatusCode int
	URL        string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d from %s", e.StatusCode, e.URL)
}

func (c *httpRegistryClient) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{StatusCode: resp.StatusCode, URL: rawURL}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxRegistryResponseBytes))
}

// osvFile extends the shared OSVAdvisory struct to include fields
// the bridge needs but the shared type omits.
type osvFile struct {
	vulnerability_parser.OSVAdvisory
	Aliases []string `json:"aliases"`
}

func parseOSVRecordsForPackage(data []byte, groupID, artifactID, baseVersion string) ([]OSVRecord, bool, error) {
	var adv osvFile
	if err := json.Unmarshal(data, &adv); err != nil {
		return nil, false, err
	}
	if !osvMatchesPackage(adv, groupID, artifactID, baseVersion) {
		return nil, false, nil
	}
	return recordsFromAdvisory(adv), true, nil
}

func parseOSVRecords(data []byte) ([]OSVRecord, error) {
	var adv osvFile
	if err := json.Unmarshal(data, &adv); err != nil {
		return nil, err
	}
	return recordsFromAdvisory(adv), nil
}

func osvMatchesPackage(adv osvFile, groupID, artifactID, baseVersion string) bool {
	backport := ""
	if adv.DatabaseSpecific != nil && adv.DatabaseSpecific.Lightwell != nil {
		backport = adv.DatabaseSpecific.Lightwell.BackportBaseVersion
	}
	for _, affected := range adv.Affected {
		if !packageMatches(affected.Package, groupID, artifactID) {
			continue
		}
		version := backport
		if len(affected.Versions) > 0 && affected.Versions[0] != "" {
			version = affected.Versions[0]
		}
		if version != "" && version == baseVersion {
			return true
		}
	}
	return false
}

func packageMatches(pkg vulnerability_parser.OSVPackage, groupID, artifactID string) bool {
	if pkg.Purl != "" {
		want := "pkg:maven/" + groupID + "/" + artifactID
		return pkg.Purl == want || strings.HasPrefix(pkg.Purl, want+"@") || strings.HasPrefix(pkg.Purl, want+"?")
	}
	return pkg.Name == artifactID || pkg.Name == groupID+":"+artifactID
}

func recordsFromAdvisory(adv osvFile) []OSVRecord {
	ids := adv.Upstream
	if len(ids) == 0 {
		ids = adv.Aliases
	}
	var cveIDs []string
	var otherAliases []string
	for _, id := range ids {
		if strings.HasPrefix(id, "CVE-") {
			cveIDs = append(cveIDs, id)
		} else if id != "" {
			otherAliases = append(otherAliases, id)
		}
	}
	if len(cveIDs) == 0 {
		if cveID := cveFromAdvisoryID(adv.ID); cveID != "" {
			cveIDs = []string{cveID}
		}
	}
	if len(cveIDs) == 0 {
		return nil
	}

	base := OSVRecord{
		Description: adv.Details,
		Aliases:     otherAliases,
	}
	for _, sev := range adv.Severity {
		if sev.Type == "CVSS_V3" {
			base.CVSSVector = sev.Score
			base.CVSSScore = computeCVSS31BaseScore(sev.Score)
			base.Severity = cvssScoreToSeverity(base.CVSSScore)
			break
		}
	}
	if base.Severity == "" {
		base.Severity = "unknown"
	}

	records := make([]OSVRecord, 0, len(cveIDs))
	for _, cveID := range cveIDs {
		rec := base
		rec.CVEID = cveID
		records = append(records, rec)
	}
	return records
}

func cveFromAdvisoryID(id string) string {
	idx := strings.Index(id, "CVE-")
	if idx < 0 {
		return ""
	}
	tail := id[idx:]
	parts := strings.SplitN(tail, "-", 4)
	if len(parts) >= 3 {
		return parts[0] + "-" + parts[1] + "-" + parts[2]
	}
	return ""
}

// computeCVSS31BaseScore calculates the CVSS 3.1 base score from a vector string.
func computeCVSS31BaseScore(vector string) float64 {
	metrics := parseCVSSVector(vector)
	if len(metrics) == 0 {
		return 0
	}

	av := cvssMetricValue(metrics, "AV", map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.20})
	ac := cvssMetricValue(metrics, "AC", map[string]float64{"L": 0.77, "H": 0.44})
	pr := cvssMetricValue(metrics, "PR", map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27})
	ui := cvssMetricValue(metrics, "UI", map[string]float64{"N": 0.85, "R": 0.62})
	s := metrics["S"]
	ci := cvssMetricValue(metrics, "C", map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0})
	ii := cvssMetricValue(metrics, "I", map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0})
	ai := cvssMetricValue(metrics, "A", map[string]float64{"H": 0.56, "L": 0.22, "N": 0.0})

	// Adjust PR for scope change
	if s == "C" {
		switch metrics["PR"] {
		case "L":
			pr = 0.68
		case "H":
			pr = 0.50
		}
	}

	iss := 1.0 - (1.0-ci)*(1.0-ii)*(1.0-ai)
	var impact float64
	if s == "C" {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}

	if impact <= 0 {
		return 0
	}

	exploitability := 8.22 * av * ac * pr * ui
	var score float64
	if s == "C" {
		score = math.Min(1.08*(impact+exploitability), 10.0)
	} else {
		score = math.Min(impact+exploitability, 10.0)
	}
	return math.Ceil(score*10) / 10
}

func parseCVSSVector(vector string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(vector, "/")
	for _, p := range parts {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}

func cvssMetricValue(metrics map[string]string, key string, values map[string]float64) float64 {
	if v, ok := metrics[key]; ok {
		if val, ok := values[v]; ok {
			return val
		}
	}
	return 0
}

func cvssScoreToSeverity(score float64) string {
	switch {
	case score >= 9.0:
		return "critical"
	case score >= 7.0:
		return "high"
	case score >= 4.0:
		return "medium"
	case score > 0:
		return "low"
	default:
		return "unknown"
	}
}

// multiCVEVEXFilename chooses the multi-CVE VEX object for cveIDs.
// Per-CVE manifest names are "{cveID}-{rawCreated}.bom.json". The raw
// build-index created string is not the OSV "published" value.
// Timestamps present for every CVE are intersected. When more than one
// remains, the lexicographically greatest is used.
func multiCVEVEXFilename(entries []vulnerability_parser.ManifestEntry, cveIDs []string) (string, error) {
	if len(cveIDs) == 0 {
		return "", fmt.Errorf("no CVE IDs")
	}

	var intersection map[string]struct{}
	for _, cveID := range cveIDs {
		re := regexp.MustCompile("^" + regexp.QuoteMeta(cveID) + `-(.+)\.bom\.json$`)
		found := make(map[string]struct{})
		for _, entry := range entries {
			match := re.FindStringSubmatch(entry.Filename)
			if match == nil {
				continue
			}
			found[match[1]] = struct{}{}
		}
		if intersection == nil {
			intersection = found
			continue
		}
		next := make(map[string]struct{})
		for ts := range found {
			if _, ok := intersection[ts]; ok {
				next[ts] = struct{}{}
			}
		}
		intersection = next
	}
	if len(intersection) == 0 {
		return "", fmt.Errorf("no shared VEX timestamp for %s", strings.Join(cveIDs, ", "))
	}

	timestamps := make([]string, 0, len(intersection))
	for ts := range intersection {
		timestamps = append(timestamps, ts)
	}
	sort.Strings(timestamps)
	chosen := timestamps[len(timestamps)-1]
	if len(timestamps) > 1 {
		log.Info().
			Strs("candidates", timestamps).
			Str("chosen", chosen).
			Msg("multiple VEX timestamps; using lexicographically greatest")
	}
	return chosen + ".bom.json", nil
}

func groupPath(groupID string) string {
	return strings.ReplaceAll(groupID, ".", "/")
}
