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
		err := renameDomain(ctx, db.DB, daoReg, config.RedHatOrg, config.RedHatDomainName)
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
			err := renameDomain(ctx, db.DB, daoReg, domain.OrgId, newName)
			if err != nil {
				renameErrors[config.RedHatOrg] = err
			}
		}
	}

	for orgId, err := range renameErrors {
		log.Error().Err(err).Msgf("Failed to rename domain %v", orgId)
	}
}

func renameDomain(ctx context.Context, DB *gorm.DB, daoReg *dao.DaoRegistry, orgId string, newName string) error {
	pulpClient := pulp_client.GetGlobalPulpClient()
	cpClient := candlepin_client.NewCandlepinClient()

	domainName, err := daoReg.Domain.Fetch(ctx, orgId)
	if err != nil {
		return fmt.Errorf("could not fetch domain name: %v", err)
	}

	rhDomanName, err := daoReg.Domain.Fetch(ctx, config.RedHatOrg)
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
		prefix, err := config.EnvironmentPrefix(pulpPath, rhDomanName, template.UUID)
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

	// Update it in pulp
	href, err := pulpClient.LookupDomain(ctx, orgId)
	if err != nil {
		return fmt.Errorf("could not lookup domain: %v", err)
	} else if href != "" {
		err = pulpClient.UpdateDomainName(ctx, domainName, newName)
		if err != nil {
			return fmt.Errorf("could not update pulp domain name: %v", err)
		}
	}

	// Complete, so update the domain name in our db
	res := DB.WithContext(ctx).Model(&models.Domain{}).Where("org_id = ?", orgId).Update("domain_name", newName)
	if res.Error != nil {
		return fmt.Errorf("could not update domain name in db: %v", res.Error)
	}
	return nil
}
