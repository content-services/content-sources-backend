package jfrog_bridge

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
	"github.com/package-url/packageurl-go"
	"github.com/rs/zerolog/log"
)

const (
	maxRegistryResponseBytes = 200 << 20 // 200 MB
	maxJFrogPingBytes        = 1 << 10   // 1 KB
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
	FetchSBOM(ctx context.Context, groupID, artifactID, version string) (bom, asc []byte, err error)
	FetchOSVRecords(ctx context.Context, groupID, artifactID, baseVersion string) ([]OSVRecord, error)
	FetchVEX(ctx context.Context, cveIDs []string, groupID, artifactID, version, serialNumber string) ([]byte, error)
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

func (c *httpRegistryClient) FetchVEX(ctx context.Context, cveIDs []string, groupID, artifactID, version, serialNumber string) ([]byte, error) {
	if len(cveIDs) == 0 {
		return nil, fmt.Errorf("fetch VEX: no CVE IDs")
	}
	uuid := strings.TrimPrefix(serialNumber, "urn:uuid:")
	if uuid == "" {
		return nil, fmt.Errorf("fetch VEX: empty SBOM serialNumber")
	}
	manifestData, err := c.getWithRetry(ctx, c.vexURL+"/PULP_MANIFEST")
	if err != nil {
		return nil, fmt.Errorf("fetch VEX: %w", err)
	}
	names := vexCandidateFilenames(vulnerability_parser.ParseManifest(manifestData), cveIDs)
	if len(names) == 0 {
		return nil, fmt.Errorf("fetch VEX: no VEX file for %s", strings.Join(cveIDs, ", "))
	}
	for _, filename := range names {
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
		ok, err := vexDocumentAccepted(data, groupID, artifactID, version, uuid)
		if err != nil {
			return nil, fmt.Errorf("fetch VEX %s: %w", filename, err)
		}
		if !ok {
			log.Debug().Str("file", filename).Msg("VEX document does not match this SBOM")
			continue
		}
		log.Info().Str("file", filename).Msg("selected VEX document")
		return data, nil
	}
	return nil, fmt.Errorf("fetch VEX: no document matches SBOM %s", uuid)
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
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
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

// vexCandidateFilenames lists VEX object names to try, newest timestamp first.
// One CVE uses the per-CVE name. Two or more CVEs use the bundled name when
// that name is in the manifest, and otherwise the per-CVE names for that timestamp.
func vexCandidateFilenames(entries []vulnerability_parser.ManifestEntry, cveIDs []string) []string {
	present := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		present[entry.Filename] = struct{}{}
	}

	var timestamps []string
	seenTS := make(map[string]struct{})
	for _, cveID := range cveIDs {
		re := regexp.MustCompile("^" + regexp.QuoteMeta(cveID) + `-(.+)\.bom\.json$`)
		for _, entry := range entries {
			match := re.FindStringSubmatch(entry.Filename)
			if match == nil {
				continue
			}
			if _, ok := seenTS[match[1]]; ok {
				continue
			}
			seenTS[match[1]] = struct{}{}
			timestamps = append(timestamps, match[1])
		}
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] > timestamps[j]
	})

	var names []string
	for _, ts := range timestamps {
		if len(cveIDs) == 1 {
			names = append(names, cveIDs[0]+"-"+ts+".bom.json")
			continue
		}
		bundled := ts + ".bom.json"
		if _, ok := present[bundled]; ok {
			names = append(names, bundled)
			continue
		}
		for _, cveID := range cveIDs {
			name := cveID + "-" + ts + ".bom.json"
			if _, ok := present[name]; ok {
				names = append(names, name)
			}
		}
	}
	return names
}

// vexDocumentAccepted reports whether the VEX body points at this SBOM.
// A download or JSON error is not a rejection: the caller must stop.
func vexDocumentAccepted(data []byte, groupID, artifactID, version, uuid string) (bool, error) {
	var doc struct {
		Metadata *struct {
			Component *struct {
				Purl string `json:"purl"`
			} `json:"component"`
		} `json:"metadata"`
		Vulnerabilities []struct {
			Affects []struct {
				Ref string `json:"ref"`
			} `json:"affects"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	needle := "urn:cdx:" + uuid + "/"
	matched := false
	for _, vuln := range doc.Vulnerabilities {
		for _, affect := range vuln.Affects {
			if strings.Contains(affect.Ref, needle) {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		return false, nil
	}
	if doc.Metadata == nil || doc.Metadata.Component == nil || doc.Metadata.Component.Purl == "" {
		return true, nil
	}
	return mavenPurlMatches(doc.Metadata.Component.Purl, groupID, artifactID, version), nil
}

func mavenPurlMatches(raw, groupID, artifactID, version string) bool {
	parsed, err := packageurl.FromString(raw)
	if err != nil || parsed.Type != "maven" || parsed.Namespace == "" || parsed.Name == "" || parsed.Version == "" {
		return false
	}
	return parsed.Namespace == groupID && parsed.Name == artifactID && parsed.Version == version
}

func groupPath(groupID string) string {
	return strings.ReplaceAll(groupID, ".", "/")
}
