package jobs

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
)

func RenameDomains() {
	ctx := context.Background()
	daoReg := dao.GetDaoRegistry(db.DB)

	var renameErrors = make(map[string]error)

	// rename the red hat domain
	rhDomain := models.Domain{}
	res := db.DB.Where("org_id = ?", config.RedHatOrg).First(&rhDomain)
	if rhDomain.DomainName != config.RedHatDomainName {
		if res.Error != nil {
			log.Error().Err(res.Error).Msg("failed to lookup RedHat domain")
		} else {
			err := daoReg.Domain.RenameDomain(ctx, config.RedHatOrg, config.RedHatDomainName)
			if err != nil {
				renameErrors[config.RedHatOrg] = err
			}
		}
	}

	customDomains := []models.Domain{}
	res = db.DB.Where("org_id != ? AND domain_name not like 'cs-%'", config.RedHatOrg).Find(&customDomains)
	if res.Error != nil {
		log.Error().Err(res.Error).Msg("failed to lookup custom domains")
	} else {
		for _, domain := range customDomains {
			err := daoReg.Domain.RenameDomain(ctx, domain.OrgId, fmt.Sprintf("cs-%v", domain.DomainName))
			if err != nil {
				renameErrors[config.RedHatOrg] = err
			}
		}
	}

	for orgId, err := range renameErrors {
		log.Error().Err(err).Msgf("Failed to rename domain %v", orgId)
	}
}
