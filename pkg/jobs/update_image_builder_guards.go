package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/rs/zerolog/log"
)

func UpdateImageBuilderGuards(_ []string) {
	orgID, username, err := config.Get().Clients.Pulp.ImageBuilderIdentity()
	if err != nil {
		log.Fatal().Err(err).Msg("invalid Image Builder TBR login")
	}
	if orgID == "" || username == "" {
		log.Fatal().Msg("clients.pulp.image_builder_username must be configured before running this job")
	}
	daoReg := dao.GetDaoRegistry(db.DB)
	if err := updateImageBuilderGuards(context.Background(), daoReg.Domain,
		pulp_client.GetGlobalPulpClient(), pulp_client.GetPulpClientWithDomain); err != nil {
		log.Fatal().Err(err).Msg("Image Builder content guard backfill failed; fix the errors and rerun")
	}
}

func updateImageBuilderGuards(ctx context.Context, domains dao.DomainDao, global pulp_client.PulpGlobalClient,
	clientForDomain func(string) pulp_client.PulpClient,
) error {
	allDomains, err := domains.List(ctx)
	if err != nil {
		return fmt.Errorf("listing Content Sources domains: %w", err)
	}

	// Partner snapshots use the Copr domain without recording it in the domains table.
	coprDomain := config.Get().Clients.Pulp.CoprDomain
	if coprDomain != "" && !slices.ContainsFunc(allDomains, func(domain models.Domain) bool {
		return domain.DomainName == coprDomain
	}) {
		allDomains = append(allDomains, models.Domain{DomainName: coprDomain})
	}

	var failures []error
	updated, skipped := 0, 0
	for _, domain := range allDomains {
		href, err := global.LookupDomain(ctx, domain.DomainName)
		if err != nil {
			failures = append(failures, fmt.Errorf("looking up domain %s: %w", domain.DomainName, err))
			continue
		}
		if href == "" {
			// CS records a domain before it is provisioned in Pulp.
			skipped++
			log.Info().Str("domain", domain.DomainName).Msg("Skipping domain not provisioned in Pulp")
			continue
		}

		count, err := addImageBuilderToContentGuards(ctx, clientForDomain(domain.DomainName))
		updated += count
		if err != nil {
			log.Error().Err(err).Str("domain", domain.DomainName).Msg("Failed to backfill domain guards")
			failures = append(failures, fmt.Errorf("updating domain %s: %w", domain.DomainName, err))
		}
	}

	log.Info().Int("domains", len(allDomains)).Int("skipped_domains", skipped).
		Int("composites_updated", updated).Int("failed_domains", len(failures)).
		Msg("Finished Image Builder content guard backfill")

	return errors.Join(failures...)
}

// addImageBuilderToContentGuards adds the configured TBR guard to existing
// Content Sources composites, preserving every current child and composite href.
func addImageBuilderToContentGuards(ctx context.Context, client pulp_client.PulpClient) (int, error) {
	allGuards, err := client.ListCompositeContentGuards(ctx)
	if err != nil {
		return 0, err
	}

	var guards []zest.CompositeContentGuardResponse
	for _, guard := range allGuards {
		if guard.Name == pulp_client.COMPOSITE_GUARD_NAME || strings.HasPrefix(guard.Name, "rhel_composite_") {
			guards = append(guards, guard)
		}
	}
	if len(guards) == 0 {
		return 0, nil
	}

	// Validate before writing. An empty composite currently permits everyone;
	// adding a child to it would unexpectedly restrict access.
	for _, guard := range guards {
		if _, err := compositeGuardChildren(guard); err != nil {
			return 0, err
		}
	}

	imageBuilderHref, err := client.CreateOrUpdateImageBuilderGuard(ctx)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, listed := range guards {
		// Preserve membership changes made since the inventory was collected.
		guard, err := client.FetchCompositeContentGuard(ctx, listed.Name)
		if err != nil {
			return updated, err
		}
		if guard == nil {
			return updated, fmt.Errorf("composite guard %q disappeared during backfill", listed.Name)
		}

		children, err := compositeGuardChildren(*guard)
		if err != nil {
			return updated, err
		}
		if slices.Contains(children, imageBuilderHref) {
			continue
		}

		children = append(children, imageBuilderHref)
		if _, err = client.UpdateCompositeGuardChildren(ctx, *guard.PulpHref, children); err != nil {
			return updated, err
		}
		log.Info().Str("domain", client.GetDomain()).Str("guard", guard.Name).
			Msg("Added Image Builder TBR authorization to composite guard")
		updated++
	}

	return updated, nil
}

func compositeGuardChildren(guard zest.CompositeContentGuardResponse) ([]string, error) {
	if guard.PulpHref == nil || *guard.PulpHref == "" || len(guard.Guards) == 0 {
		return nil, fmt.Errorf("composite guard %q has no href or children; refusing to change authorization", guard.Name)
	}

	children := make([]string, 0, len(guard.Guards))
	for _, child := range guard.Guards {
		if child == nil || *child == "" {
			return nil, fmt.Errorf("composite guard %q has an invalid child href", guard.Name)
		}
		children = append(children, *child)
	}

	return children, nil
}
