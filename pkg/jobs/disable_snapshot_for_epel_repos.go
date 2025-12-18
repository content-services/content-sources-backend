package jobs

import (
	"os"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
)

func getSkipIdList() []string {
	env := os.Getenv("EPEL_ORG_ID_SKIP")
	if env == "" {
		// Require this skip list be set to prevent it from running without it
		log.Fatal().Msg("EPEL_ORG_ID_SKIP environment variable not set")
	}
	split := strings.Split(env, ",")
	split = append(split, config.CommunityOrg)
	split = append(split, config.RedHatOrg)
	return split
}

func DisableSnapshotForEpelRepos(_ []string) {
	skipOrgIdList := getSkipIdList()

	// EPEL repository URLs to match
	epelURLs := []string{
		"https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/",
		"https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/",
		"https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/",
	}

	// Clean up URLs (add trailing slash, remove leading/trailing whitespace)
	cleanedURLs := make([]string, len(epelURLs))
	for i, url := range epelURLs {
		cleanedURLs[i] = models.CleanupURL(url)
	}

	repoConfigs := []models.RepositoryConfiguration{}

	selectQuery := `
		select rc.org_id as org_id, rc.uuid as uuid from repository_configurations rc 
		   inner join repositories r on rc.repository_uuid = r.uuid 
		   where r.url in (?) and 
			 rc.org_id not in (select org_id from templates) and 
			 rc.snapshot = true and rc.org_id not in (?)
`
	res := db.DB.Raw(selectQuery, cleanedURLs, skipOrgIdList).Find(&repoConfigs)
	if res.Error != nil {
		log.Fatal().Err(res.Error)
	}
	for _, repoConfig := range repoConfigs {
		log.Warn().Msgf("Updating org: %v, repository: %v", repoConfig.OrgID, repoConfig.UUID)
	}

	updateQuery := `
		UPDATE repository_configurations rc 
			SET snapshot = false
		   FROM repositories r 
		   WHERE rc.repository_uuid = r.uuid 
			 AND r.url in (?) and 
			 rc.org_id not in (select org_id from templates) and 
			 rc.snapshot = true and rc.org_id not in (?)
			
`
	res = db.DB.Exec(updateQuery, cleanedURLs, skipOrgIdList)
	if res.Error != nil {
		log.Fatal().Err(res.Error).Msg("failed to update snapshots")
	}
	log.Warn().Msgf("Updated repos: %v", res.RowsAffected)
}
