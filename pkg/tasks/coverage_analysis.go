package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/catalog"
	"github.com/content-services/content-sources-backend/pkg/coverage/cve"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/content-services/content-sources-backend/pkg/coverage/parser"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/rs/zerolog/log"
)

type CoverageAnalysis struct {
	ctx        context.Context
	payload    *payloads.CoverageAnalysisPayload
	daoReg     *dao.DaoRegistry
	s3Client   s3_client.S3Client
	pulpClient pulp_client.PulpClient
	tang       tangy.Tangy
}

// CoverageAnalysisHandler processes an uploaded manifest against the validated
// package catalog and persists the resulting coverage analysis.
func CoverageAnalysisHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := payloads.CoverageAnalysisPayload{}
	err := json.Unmarshal(task.Payload, &opts)
	if err != nil {
		return fmt.Errorf("payload incorrect type for %s", config.CoverageAnalysisTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.LightwellOrg)
	if err != nil {
		return err
	}

	s3Client, err := s3_client.NewS3Client(config.Get().Clients.Lightwell.S3.CoverageUploads)
	if err != nil {
		return fmt.Errorf("failed to initialize s3 client: %w", err)
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	if config.Tang == nil {
		return fmt.Errorf("no tang configuration present")
	}

	ca := CoverageAnalysis{
		ctx:        ctxWithLogger,
		payload:    &opts,
		daoReg:     daoReg,
		s3Client:   s3Client,
		pulpClient: pulpClient,
		tang:       *config.Tang,
	}
	err = ca.Run()
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	msg := err.Error()
	updateErr := daoReg.CoverageReport.UpdateCoverageReportStatus(context.Background(), opts.CoverageReportUUID, config.TaskStatusFailed, &msg)
	if updateErr != nil {
		log.Error().Errs("errors", []error{err, updateErr}).Str("coverage_report_uuid", opts.CoverageReportUUID).Msg("failed to update coverage report status")
	}
	return err
}

func (c *CoverageAnalysis) Run() error {
	upload, err := c.daoReg.CoverageReport.InternalOnlyFetchCoverageUpload(c.ctx, c.payload.CoverageUploadUUID)
	if err != nil {
		return fmt.Errorf("failed to fetch upload: %w", err)
	}

	body, err := c.s3Client.Get(c.ctx, upload.StorageKey)
	if err != nil {
		return fmt.Errorf("failed to download manifest from S3: %w", err)
	}
	defer body.Close()

	// Stream the manifest instead of buffering it in memory.
	// The extra byte lets us detect an S3 object that is larger than the stored upload size.
	limit := upload.SizeBytes + 1
	limited := &io.LimitedReader{
		R: body,
		N: limit,
	}

	// Hash the manifest as the parser consumes it
	hash := sha256.New()
	reader := io.TeeReader(limited, hash)

	parsedPackages, parseErr := parser.Parse(c.payload.Filename, reader)

	// Ensure the entire object was consumed and hashed before checking parse errors
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("failed to finish reading manifest: %w", err)
	}

	sizeBytes := limit - limited.N
	if sizeBytes != upload.SizeBytes {
		return fmt.Errorf("manifest size mismatch: expected %d, got %d", upload.SizeBytes, sizeBytes)
	}

	// Verify the manifest fetched is identical to the original upload
	// before using any results produced by the parser
	if hex.EncodeToString(hash.Sum(nil)) != upload.Sha256 {
		return fmt.Errorf("manifest sha256 mismatch")
	}

	if parseErr != nil {
		return fmt.Errorf("failed to parse manifest: %w", parseErr)
	}

	if len(parsedPackages.Packages) == 0 {
		if parsedPackages.SkippedEntries > 0 {
			//nolint:staticcheck // ST1005: user-facing message persisted verbatim to AnalysisTaskError and rendered in the UI; never wrapped further
			return fmt.Errorf(
				"None of the %d package entries in the uploaded %s manifest include a Package URL (PURL).\n"+
					"Coverage analysis requires PURLs to identify packages — consider regenerating the SBOM with a PURL-aware tool such as Syft.",
				parsedPackages.SkippedEntries, parsedPackages.InputFormat,
			)
		}
		return fmt.Errorf("no packages found in manifest")
	}

	err = c.daoReg.CoverageReport.UpdateCoverageReportStatus(c.ctx, c.payload.CoverageReportUUID, config.TaskStatusRunning, nil)
	if err != nil {
		return fmt.Errorf("failed to update coverage report status: %w", err)
	}

	validatedCatalog, snapshotAt, err := catalog.LoadCatalog(c.ctx, c.daoReg, c.pulpClient, c.tang)
	if err != nil {
		return fmt.Errorf("failed to load catalog: %w", err)
	}

	results, summary := matcher.MatchCatalog(validatedCatalog, toMatcherPackages(parsedPackages.Packages), snapshotAt)

	// CVE enrichment is best-effort: a failure to load advisories should not fail the
	// whole analysis. On error we persist the coverage results without CVE data, leaving
	// the per-package and summary CVE counts at their zero defaults.
	var packageCVEs []cve.PackageCVE
	var cveSummary cve.Count
	advisoryFilter := toRemediatedAdvisoryFilter(cve.ExactMatchKeys(results))
	advisories, err := c.daoReg.LightwellAdvisory.ListRemediatedAdvisories(c.ctx, config.LightwellOrg, advisoryFilter)
	if err != nil {
		log.Ctx(c.ctx).Error().Err(err).Msg("failed to load remediated advisories; continuing without CVE data")
	} else {
		packageCVEs, cveSummary = cve.Enrich(results, toCVEAdvisories(advisories))
	}

	err = c.daoReg.CoverageReport.SaveCoverageAnalysis(c.ctx, c.payload.CoverageReportUUID, dao.SaveCoverageAnalysisParams{
		InputFormat: parsedPackages.InputFormat,
		Results:     results,
		PackageCVEs: packageCVEs,
		CveSummary:  cveSummary,
		Summary:     summary,
	})
	if err != nil {
		return fmt.Errorf("failed to save coverage analysis: %w", err)
	}
	return nil
}

// toRemediatedAdvisoryFilter maps exact-match package keys (grouped by matcher ecosystem)
// to the DAO filter so only advisories for those packages are fetched from the catalog.
func toRemediatedAdvisoryFilter(keys map[string][]string) dao.RemediatedAdvisoryFilter {
	return dao.RemediatedAdvisoryFilter{
		MavenPackageKeys:  keys[matcher.EcosystemJava],
		PythonPackageKeys: keys[matcher.EcosystemPython],
	}
}

// toCVEAdvisories converts remediated advisories into the enricher's input, mapping the
// repository content type to the matcher ecosystem used for package-name normalization.
func toCVEAdvisories(advisories []dao.RemediatedAdvisory) []cve.Advisory {
	out := make([]cve.Advisory, 0, len(advisories))
	for _, advisory := range advisories {
		ecosystem := ecosystemForContentType(advisory.ContentType)
		if ecosystem == "" {
			continue
		}
		out = append(out, cve.Advisory{
			Ecosystem:      ecosystem,
			PackageName:    advisory.PackageName,
			AdvisoryID:     advisory.AdvisoryID,
			SeverityScore:  advisory.SeverityScore,
			PackageVersion: advisory.PackageVersion,
		})
	}
	return out
}

func ecosystemForContentType(contentType string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return matcher.EcosystemJava
	case config.ContentTypePython:
		return matcher.EcosystemPython
	default:
		return ""
	}
}

func toMatcherPackages(pkgs []parser.Package) []matcher.Package {
	out := make([]matcher.Package, len(pkgs))
	for i, pkg := range pkgs {
		out[i] = matcher.Package{
			Ecosystem: pkg.Ecosystem,
			Name:      pkg.Name,
			Version:   pkg.Version,
			Namespace: pkg.Namespace,
		}
	}
	return out
}
