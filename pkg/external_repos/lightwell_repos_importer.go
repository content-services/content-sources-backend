package external_repos

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/rs/zerolog/log"
)

//go:embed "lightwell_repos.json"
var lightwellFS embed.FS

//go:embed "lightwell_demo_repos.json"
var lightwellDemoFS embed.FS

type LightwellAllowlistEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	BasePath    string `json:"base_path"`
	FeatureName string `json:"feature_name"`
}

type LightwellRepoImporter struct {
	daoReg         *dao.DaoRegistry
	pulpClient     pulp_client.PulpClient
	demoPulpClient pulp_client.PulpClient
}

func NewLightwellRepoImporter(daoReg *dao.DaoRegistry, pulpClient pulp_client.PulpClient, demoPulpClient pulp_client.PulpClient) LightwellRepoImporter {
	return LightwellRepoImporter{
		daoReg:         daoReg,
		pulpClient:     pulpClient,
		demoPulpClient: demoPulpClient,
	}
}

func (lri *LightwellRepoImporter) LoadAndSave(ctx context.Context) error {
	entries, err := loadLightwellAllowlist()
	if err != nil {
		return fmt.Errorf("error loading lightwell allowlist: %w", err)
	}
	err = lri.importEntries(ctx, entries, lri.pulpClient, config.LightwellOrg)
	if err != nil {
		return err
	}

	if config.Get().Options.LoadLightwellDemo {
		demoEntries, err := loadLightwellDemoAllowlist()
		if err != nil {
			return fmt.Errorf("error loading lightwell demo allowlist: %w", err)
		}
		return lri.importEntries(ctx, demoEntries, lri.demoPulpClient, config.LightwellDemoOrg)
	}
	return nil
}

func (lri *LightwellRepoImporter) importEntries(ctx context.Context, entries []LightwellAllowlistEntry, pulpClient pulp_client.PulpClient, orgID string) error {
	for _, entry := range entries {
		securityLevel, err := getSecurityLevel(entry.BasePath)
		if err != nil {
			log.Warn().Err(err).Str("name", entry.Name).Msg("Skipping lightwell repository with unparseable base path")
			continue
		}

		publishedDistURL := ""
		dist, err := pulpClient.FindGenericDistributionByBasePath(ctx, entry.BasePath)
		if err != nil {
			log.Warn().Err(err).Str("name", entry.Name).Msg("Error fetching distribution for lightwell repository")
		} else if dist != nil {
			publishedDistURL = dist.GetBaseUrl()
		}

		if publishedDistURL == "" {
			return fmt.Errorf("lightwell repo %q has no published distribution URL", entry.Name)
		}

		_, err = lri.daoReg.RepositoryConfig.InternalOnly_RefreshLightwellRepo(
			ctx, orgID, entry.Name, securityLevel, entry.Type, publishedDistURL, entry.BasePath, entry.FeatureName,
		)
		if err != nil {
			return fmt.Errorf("failed to save lightwell repository %q: %w", entry.Name, err)
		}
	}
	return nil
}

func getSecurityLevel(basePath string) (securityLevel string, err error) {
	parts := strings.SplitN(basePath, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("expected format ecosystem/securitylevel, got %q", basePath)
	}
	return parts[1], nil
}

func loadLightwellAllowlist() ([]LightwellAllowlistEntry, error) {
	contents, err := lightwellFS.ReadFile("lightwell_repos.json")
	if err != nil {
		return nil, err
	}
	var entries []LightwellAllowlistEntry
	err = json.Unmarshal(contents, &entries)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func loadLightwellDemoAllowlist() ([]LightwellAllowlistEntry, error) {
	contents, err := lightwellDemoFS.ReadFile("lightwell_demo_repos.json")
	if err != nil {
		return nil, err
	}
	var entries []LightwellAllowlistEntry
	err = json.Unmarshal(contents, &entries)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
