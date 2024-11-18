package jobs

import (
	"context"
	"fmt"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func RenameDomains() {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	var renameErrors = make(map[string]error)

	// rename the red hat domain
	rhDomain := models.Domain{}
	res := db.DB.Where("org_id = ?", config.RedHatOrg).First(&rhDomain)
	if res.Error != nil {
		log.Error().Err(res.Error).Msg("failed to lookup RedHat domain")
	} else {
		err := RenameDomain(ctx, db.DB, daoReg, config.RedHatOrg, config.RedHatDomainName)
		if err != nil {
			renameErrors[config.RedHatOrg] = err
		}
	}

	customDomains := []models.Domain{}
	res = db.DB.Where("org_id != ?", config.RedHatOrg).Find(&customDomains)
	if res.Error != nil {
		log.Error().Err(res.Error).Msg("failed to lookup custom domains")
	} else {
		for _, domain := range customDomains {
			newName := domain.DomainName
			if !strings.HasPrefix(newName, "cs-") {
				newName = fmt.Sprintf("cs-%s", newName)
			}
			err := RenameDomain(ctx, db.DB, daoReg, domain.OrgId, newName)
			if err != nil {
				renameErrors[config.RedHatOrg] = err
			}
		}
	}

	for orgId, err := range renameErrors {
		log.Error().Err(err).Msgf("Failed to rename domain %v", orgId)
	}
}

func fixSnapshotPaths(ctx context.Context, DB *gorm.DB, orgId string, newDomainName string) error {
	err := DB.Debug().Exec("UPDATE snapshots SET repository_path = (? || '/' || distribution_path) "+
		"FROM repository_configurations   "+
		"WHERE snapshots.repository_configuration_uuid = repository_configurations.uuid "+
		"AND repository_configurations.org_id = ?", newDomainName, orgId).Error
	if err != nil {
		return fmt.Errorf("fix snapshot paths: %w", err)
	}
	return nil
}

func RenameDomain(ctx context.Context, DB *gorm.DB, daoReg *dao.DaoRegistry, orgId string, newName string) error {
	pulpClient := pulp_client.GetGlobalPulpClient()
	cpClient := candlepin_client.NewCandlepinClient()

	domainName, err := daoReg.Domain.Fetch(ctx, orgId)
	if err != nil {
		return fmt.Errorf("could not fetch domain name: %v", err)
	}

	rhDomainName, err := daoReg.Domain.Fetch(ctx, config.RedHatOrg)
	if err != nil {
		return fmt.Errorf("could not fetch rh domain name: %v", err)
	}

	templates, _, err := daoReg.Template.List(ctx, orgId, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	if err != nil {
		return fmt.Errorf("could not list templates for org: %v", err)
	}
	pulpPath, err := pulpClient.GetContentPath(ctx)
	if err != nil {
		return fmt.Errorf("could not get pulp path: %v", err)
	}
	for _, template := range templates.Data {
		prefix, err := config.EnvironmentPrefix(pulpPath, rhDomainName, template.UUID)
		if err != nil {
			return fmt.Errorf("could not get environment prefix: %v", err)
		}

		env, err := cpClient.FetchEnvironment(ctx, template.UUID)
		if err != nil {
			return fmt.Errorf("could not fetch environment: %v", err)
		}
		if env != nil {
			_, err = cpClient.UpdateEnvironmentPrefix(ctx, template.UUID, prefix)
			if err != nil {
				return fmt.Errorf("could not update environment prefix: %v", err)
			}
		}

		err = tasks.RemoveUneededOverrides(ctx, cpClient, template.UUID, []caliri.ContentOverrideDTO{})
		if err != nil {
			return fmt.Errorf("could not clear overrides for update: %v", err)
		}

		overrideDtos, err := tasks.GenOverrideDTO(ctx, daoReg, orgId, newName, pulpPath, template)
		if err != nil {
			return fmt.Errorf("could not generate override: %v", err)
		}
		err = tasks.RemoveUneededOverrides(ctx, cpClient, template.UUID, overrideDtos)
		if err != nil {
			return err
		}

		err = cpClient.UpdateContentOverrides(ctx, template.UUID, overrideDtos)
		if err != nil {
			return err
		}
	}

	// Update snapshot paths
	err = fixSnapshotPaths(ctx, DB, orgId, newName)
	if err != nil {
		return err
	}

	// Update it in pulp
	href, err := pulpClient.LookupDomain(ctx, domainName)
	if err != nil {
		return fmt.Errorf("could not lookup domain: %v", err)
	} else if href != "" {
		err = pulpClient.UpdateDomainName(ctx, domainName, newName)
		if err != nil {
			return fmt.Errorf("could not update pulp domain name: %v", err)
		}
	}

	// Update the hrefs
	errs := renameSnapshotHrefs(ctx, DB, orgId, newName)
	for _, err := range errs {
		log.Error().Err(err).Msgf("Failed to rename href %v", orgId)
	}
	errs = renameTemplateRepoConfigHrefs(ctx, DB, orgId, newName)
	for _, err := range errs {
		log.Error().Err(err).Msgf("Failed to rename href %v", orgId)
	}

	// Complete, so update the domain name in our db
	res := DB.WithContext(ctx).Model(&models.Domain{}).Where("org_id = ?", orgId).Update("domain_name", newName)
	if res.Error != nil {
		return fmt.Errorf("could not update domain name in db: %v", res.Error)
	}
	return nil
}

func renameTemplateRepoConfigHrefs(ctx context.Context, DB *gorm.DB, orgId string, newDomainName string) []error {
	errs := []error{}
	trcs := []models.TemplateRepositoryConfiguration{}
	res := DB.WithContext(ctx).Joins("INNER JOIN repository_configurations on templates_repository_configurations.repository_configuration_uuid = repository_configurations.uuid").
		Where(" repository_configurations.org_id = ?", orgId).Find(&trcs)
	if res.Error != nil {
		return []error{fmt.Errorf("could not find template repo config references: %v", res.Error)}
	}
	for _, trc := range trcs {
		newHref, err := ChangeHrefDomain(trc.DistributionHref, newDomainName)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not calculate distribution href %v, %v, '%v': %w",
				trc.TemplateUUID, trc.RepositoryConfigurationUUID, trc.DistributionHref, err))
		}
		if trc.DistributionHref != newHref {
			trc.DistributionHref = newHref
			res = DB.WithContext(ctx).Where("template_uuid = ? and repository_configuration_uuid = ?", trc.TemplateUUID, trc.RepositoryConfigurationUUID).Updates(&trc)
			if res.Error != nil {
				errs = append(errs, fmt.Errorf("could not rename template repo config references %v, %v: %w",
					trc.TemplateUUID, trc.RepositoryConfigurationUUID, err))
			}
		}
	}
	return errs
}

func renameSnapshotHrefs(ctx context.Context, DB *gorm.DB, orgId string, newDomainName string) []error {
	errs := []error{}

	snaps := []models.Snapshot{}
	res := DB.WithContext(ctx).Joins("INNER JOIN repository_configurations on snapshots.repository_configuration_uuid = repository_configurations.uuid").
		Where(" repository_configurations.org_id = ?", orgId).Find(&snaps)
	if res.Error != nil {
		return []error{fmt.Errorf("could not find snapshot references: %v", res.Error)}
	}
	for _, snap := range snaps {
		oldSnap := snap
		newVersion, err := ChangeHrefDomain(snap.VersionHref, newDomainName)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not rename snapshot (%v) version_href (%v): %v", snap.UUID, snap.VersionHref, err))
		}
		snap.VersionHref = newVersion

		newPublication, err := ChangeHrefDomain(snap.PublicationHref, newDomainName)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not rename snapshot (%v) publication_href (%v): %v", snap.UUID, snap.PublicationHref, err))
		}
		snap.PublicationHref = newPublication

		newDistribution, err := ChangeHrefDomain(snap.DistributionHref, newDomainName)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not rename snapshot (%v) distribution_href (%v): %v", snap.UUID, snap.DistributionHref, err))
		}
		snap.DistributionHref = newDistribution

		if oldSnap.DistributionHref != snap.DistributionHref ||
			oldSnap.VersionHref != snap.VersionHref ||
			oldSnap.PublicationHref != snap.PublicationHref {
			res = DB.WithContext(ctx).Updates(&snap)
			if res.Error != nil {
				errs = append(errs, fmt.Errorf("could not updates napshot model %v: %v", snap.UUID, err))
			}
		}
	}
	return errs
}

func ChangeHrefDomain(href, newDomain string) (newHref string, err error) {
	splitPath := strings.Split(href, "/")
	if len(splitPath) < 4 {
		return newHref, fmt.Errorf("invalid href format (%v)", href)
	}
	splitPath[3] = newDomain
	newHref = strings.Join(splitPath, "/")
	return newHref, err
}
