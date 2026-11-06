package copr_ingest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/rs/zerolog"
)

const BuildEndEventType = "copr.build.end"

// buildEndData holds the fields we use from the data of a Copr build end CloudEvent.
type buildEndData struct {
	// RepositoryHref uniquely identifies the Copr Pulp repository
	RepositoryHref  string `json:"repository_href"`
	PublicationHref string `json:"publication_href"`
	Chroot          string `json:"chroot"`
	Build           int64  `json:"build"`
	Owner           string `json:"owner"`
	Copr            string `json:"copr"`
	Status          *int   `json:"status"`
}

func (d buildEndData) succeeded() bool {
	return d.Status != nil && *d.Status == 1 // Copr build status: 1 succeeded, 0 failed
}

// parseBuildEndData decodes and validates the Copr build end data from the CloudEvent.
func parseBuildEndData(ce cloudevents.Event) (buildEndData, error) {
	var data buildEndData
	if err := ce.DataAs(&data); err != nil {
		return buildEndData{}, fmt.Errorf("%w: invalid build end data: %w", errInvalidMessage, err)
	}
	if data.Status == nil {
		return buildEndData{}, fmt.Errorf("%w: invalid build end data: missing status", errInvalidMessage)
	}
	// Failed builds are skipped, so the fields needed for the snapshot task aren't required for them
	if !data.succeeded() {
		return data, nil
	}

	var missing []string
	if data.RepositoryHref == "" {
		missing = append(missing, "repository_href")
	}
	if data.PublicationHref == "" {
		missing = append(missing, "publication_href")
	}
	if data.Chroot == "" {
		missing = append(missing, "chroot")
	}
	if data.Build <= 0 {
		missing = append(missing, "build")
	}
	if data.Owner == "" {
		missing = append(missing, "owner")
	}
	if data.Copr == "" {
		missing = append(missing, "copr")
	}
	if len(missing) > 0 {
		return buildEndData{}, fmt.Errorf("%w: invalid build end data: missing %s", errInvalidMessage, strings.Join(missing, ", "))
	}
	return data, nil
}

// handleBuildEnd skips failed builds and unsupported chroots, and enqueues a snapshot task for the rest.
func (h *BuildEndHandler) handleBuildEnd(ctx context.Context, eventID string, data buildEndData) error {
	logger := zerolog.Ctx(ctx)
	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("copr_build_id", data.Build).Str("chroot", data.Chroot)
	})
	if !data.succeeded() {
		logger.Debug().Int("status", *data.Status).Msg("skipping unsuccessful build")
		return nil
	}
	distro, version, arch, err := parseChroot(data.Chroot)
	if err != nil {
		return err
	}
	if !supportedChroot(distro, version, arch) {
		logger.Debug().Msg("skipping unsupported chroot")
		return nil
	}
	payload, err := toSnapshotTaskPayload(data, distro, version, arch)
	if err != nil {
		return err
	}
	if err := h.enqueue(ctx, eventID, payload); err != nil {
		return err
	}
	h.metrics.RecordCoprBuildEndResult(instrumentation.CoprBuildEndResultEnqueued)
	return nil
}

const maxRepositoryNameLength = 255 // length limit for repository_configurations name column

// toSnapshotTaskPayload maps a supported build-end chroot to a snapshot task payload.
func toSnapshotTaskPayload(data buildEndData, distro, version, arch string) (payloads.CreatePartnerSnapshotPayload, error) {
	projectID := data.Owner + "/" + data.Copr
	// The owner is part of the name, since repository names are unique per org and project names are only unique per owner
	name := fmt.Sprintf("%s for %s %s %s", projectID, config.CoprChrootDistros[distro], version, arch) // e.g., "@theforeman/foreman-nightly-staging for RHEL 9 aarch64"
	if len(name) > maxRepositoryNameLength {
		return payloads.CreatePartnerSnapshotPayload{}, fmt.Errorf("%w: repository name %q is longer than %d characters",
			errInvalidMessage, name, maxRepositoryNameLength)
	}
	return payloads.CreatePartnerSnapshotPayload{
		PublicationHref:    data.PublicationHref,
		RepositoryName:     name,
		RepositoryVersion:  version,
		RepositoryArch:     arch,
		CoprProjectID:      projectID,
		CoprRepositoryHref: data.RepositoryHref,
		CoprBuildID:        strconv.FormatInt(data.Build, 10),
	}, nil
}

// parseChroot splits "<distro>-<version>-<arch>", taking version and arch from the end.
func parseChroot(chroot string) (distro, version, arch string, err error) {
	parts := strings.Split(chroot, "-")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("%w: invalid chroot %q, expected <distro>-<version>-<arch>", errInvalidMessage, chroot)
	}
	if slices.Contains(parts, "") {
		return "", "", "", fmt.Errorf("%w: invalid chroot %q, expected <distro>-<version>-<arch>", errInvalidMessage, chroot)
	}
	n := len(parts)
	return strings.Join(parts[:n-2], "-"), parts[n-2], parts[n-1], nil
}

// supportedChroot reports whether distro, version, and arch are allowed by Content Sources.
func supportedChroot(distro, version, arch string) bool {
	if _, ok := config.CoprChrootDistros[distro]; !ok {
		return false
	}
	validVersion, _ := config.ValidDistributionVersionLabels([]string{version})
	return validVersion && version != config.ANY_VERSION && config.ValidArchLabel(arch) && arch != config.ANY_ARCH
}
