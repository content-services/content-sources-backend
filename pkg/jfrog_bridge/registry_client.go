package jfrog_bridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/external_repos/vulnerability_parser"
	"github.com/rs/zerolog/log"
)

// OSVRecord holds the bridge-internal representation of a single CVE
// extracted from an OSV JSON file.
type OSVRecord struct {
	CVEID       string
	Aliases     []string
	Description string
	CVSSVector  string
	CVSSScore   float64
	Severity    string
}

// RegistryClient abstracts HTTP calls to the Lightwell registry on
// packages.redhat.com to fetch JARs, POMs, and OSV advisory records.
type RegistryClient interface {
	FetchJAR(ctx context.Context, groupID, artifactID, version string) (body []byte, sha256hex string, err error)
	FetchPOM(ctx context.Context, groupID, artifactID, version string) ([]byte, error)
	FetchOSVRecords(ctx context.Context, baseVersion string) ([]OSVRecord, error)
}

type httpRegistryClient struct {
	httpClient  *http.Client
	registryURL string
	osvURL      string
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

func (c *httpRegistryClient) FetchOSVRecords(ctx context.Context, baseVersion string) ([]OSVRecord, error) {
	manifestURL := c.osvURL + "/PULP_MANIFEST"
	manifestData, err := c.getWithRetry(ctx, manifestURL)
	if err != nil {
		return nil, fmt.Errorf("fetch PULP_MANIFEST: %w", err)
	}

	entries := vulnerability_parser.ParseManifest(manifestData)
	var records []OSVRecord
	for _, entry := range entries {
		if !strings.Contains(entry.Filename, baseVersion) {
			continue
		}
		fileURL := c.osvURL + "/" + entry.Filename
		data, err := c.getWithRetry(ctx, fileURL)
		if err != nil {
			log.Warn().Err(err).Str("file", entry.Filename).Msg("skipping OSV file")
			continue
		}
		rec, err := parseOSVFile(data)
		if err != nil {
			log.Warn().Err(err).Str("file", entry.Filename).Msg("skipping malformed OSV")
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

func (c *httpRegistryClient) getWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		data, err := c.doGet(ctx, url)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *httpRegistryClient) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// osvFile extends the shared OSVAdvisory struct to include fields
// the bridge needs but the shared type omits.
type osvFile struct {
	vulnerability_parser.OSVAdvisory
	Aliases []string `json:"aliases"`
}

func parseOSVFile(data []byte) (OSVRecord, error) {
	var adv osvFile
	if err := json.Unmarshal(data, &adv); err != nil {
		return OSVRecord{}, err
	}

	rec := OSVRecord{
		Description: adv.Details,
	}

	// Extract CVE ID from aliases (first entry starting with "CVE-")
	var cveID string
	var otherAliases []string
	for _, alias := range adv.Aliases {
		if strings.HasPrefix(alias, "CVE-") && cveID == "" {
			cveID = alias
		} else {
			otherAliases = append(otherAliases, alias)
		}
	}
	if cveID == "" {
		// Fall back: extract CVE from the id field (e.g. "x_RHLW-CVE-2023-20860-5.3.18")
		if idx := strings.Index(adv.ID, "CVE-"); idx >= 0 {
			tail := adv.ID[idx:]
			parts := strings.SplitN(tail, "-", 4)
			if len(parts) >= 3 {
				cveID = parts[0] + "-" + parts[1] + "-" + parts[2]
			}
		}
		if cveID == "" {
			cveID = adv.ID
		}
	}
	rec.CVEID = cveID
	rec.Aliases = otherAliases

	for _, sev := range adv.Severity {
		if sev.Type == "CVSS_V3" {
			rec.CVSSVector = sev.Score
			rec.CVSSScore = computeCVSS31BaseScore(sev.Score)
			rec.Severity = cvssScoreToSeverity(rec.CVSSScore)
			break
		}
	}
	if rec.Severity == "" {
		rec.Severity = "unknown"
	}

	return rec, nil
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

func groupPath(groupID string) string {
	return strings.ReplaceAll(groupID, ".", "/")
}
