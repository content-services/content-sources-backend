package copr_ingest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/rs/zerolog"
)

const (
	BuildEndEventType       = "copr.build.end"
	maxRepositoryNameLength = 255 // length limit for repository_configurations name column
)

// TODO: add the partner org ID and Pulp domain fields once Copr sends them, until then both come from config
type buildEndData struct {
	PublicationHref string `json:"publication_href"` // the snapshot task resolves the Copr Pulp repository from it
	Chroot          string `json:"chroot"`
	Build           int64  `json:"build"`
	Owner           string `json:"owner"`
	Copr            string `json:"copr"`
	Status          *int   `json:"status"`
}

type validBuildEnd struct {
	PublicationHref string
	Build           int64
	Owner           string
	Copr            string
	Distro          string
	Version         string
	Arch            string
}

// parseValidBuildEnd checks that a successful build end has the fields needed for the snapshot task and a well-formed chroot.
func parseValidBuildEnd(d buildEndData) (validBuildEnd, error) {
	var missing []string
	if d.PublicationHref == "" {
		missing = append(missing, "publication_href")
	}
	if d.Chroot == "" {
		missing = append(missing, "chroot")
	}
	if d.Build <= 0 {
		missing = append(missing, "build")
	}
	if d.Owner == "" {
		missing = append(missing, "owner")
	}
	if d.Copr == "" {
		missing = append(missing, "copr")
	}
	if len(missing) > 0 {
		return validBuildEnd{}, fmt.Errorf("%w: invalid build end data: missing %s", errInvalidMessage, strings.Join(missing, ", "))
	}
	distro, version, arch, err := parseChroot(d.Chroot)
	if err != nil {
		return validBuildEnd{}, err
	}
	return validBuildEnd{
		PublicationHref: d.PublicationHref,
		Build:           d.Build,
		Owner:           d.Owner,
		Copr:            d.Copr,
		Distro:          distro,
		Version:         version,
		Arch:            arch,
	}, nil
}

// handleBuildEnd rejects malformed build ends, skips failed builds and unsupported chroots, and enqueues a snapshot task for the rest.
func (h *BuildEndHandler) handleBuildEnd(ctx context.Context, eventID string, data buildEndData) error {
	logger := zerolog.Ctx(ctx)
	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("copr_build_id", data.Build).Str("chroot", data.Chroot)
	})
	if data.Status == nil {
		return fmt.Errorf("%w: invalid build end data: missing status", errInvalidMessage)
	}
	if *data.Status != 1 { // Copr build status: 1 succeeded, 0 failed
		logger.Debug().Int("status", *data.Status).Msg("skipping unsuccessful build")
		return nil
	}
	build, err := parseValidBuildEnd(data)
	if err != nil {
		return err
	}
	if !supportedChroot(build.Distro, build.Version, build.Arch) {
		logger.Debug().Msg("skipping unsupported chroot")
		return nil
	}
	payload, err := toSnapshotTaskPayload(build)
	if err != nil {
		return err
	}
	if err := h.enqueue(ctx, eventID, payload); err != nil {
		return err
	}
	h.metrics.RecordCoprBuildEndResult(instrumentation.CoprBuildEndResultEnqueued)
	return nil
}

// toSnapshotTaskPayload maps a validated build to a snapshot task payload.
func toSnapshotTaskPayload(build validBuildEnd) (payloads.CreatePartnerSnapshotPayload, error) {
	projectID := build.Owner + "/" + build.Copr
	// The owner is part of the name, since repository names are unique per org and project names are only unique per owner
	name := fmt.Sprintf("%s for %s %s %s", projectID, config.CoprChrootDistros[build.Distro], build.Version, build.Arch) // e.g., "@nvidia/cuda for RHEL 9 aarch64"
	if len(name) > maxRepositoryNameLength {
		return payloads.CreatePartnerSnapshotPayload{}, fmt.Errorf("%w: repository name %q is longer than %d characters",
			errInvalidMessage, name, maxRepositoryNameLength)
	}
	return payloads.CreatePartnerSnapshotPayload{
		PublicationHref:   build.PublicationHref,
		RepositoryName:    name,
		RepositoryVersion: build.Version,
		RepositoryArch:    build.Arch,
		CoprProjectID:     projectID,
		CoprBuildID:       strconv.FormatInt(build.Build, 10),
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
