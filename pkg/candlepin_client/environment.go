package candlepin_client

import (
	"context"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/openlyinc/pointy"
)

const ENVIRONMENT_TYPE = "content-template"

func GetEnvironmentID(templateUUID string) string {
	return strings.Replace(templateUUID, "-", "", -1)
}

func (c *cpClientImpl) AssociateEnvironment(ctx context.Context, _ string, templateName string, consumerUuid string) error {
	ctx, client, err := getCandlepinClient(ctx)

	if err != nil {
		return err
	}
	envs := []caliri.EnvironmentDTO{}
	if templateName != "" {
		tempName := GetEnvironmentID(templateName)
		envs = []caliri.EnvironmentDTO{{Id: &tempName}}
	}

	httpResp, err := client.ConsumerAPI.UpdateConsumer(ctx, consumerUuid).ConsumerDTO(caliri.ConsumerDTO{Environments: envs}).Execute()
	// env, httpResp, err := client.OwnerAPI.CreateEnv(ctx, ownerKey).EnvironmentDTO(caliri.EnvironmentDTO{Id: &id, Name: &name, ContentPrefix: &prefix}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't add consumer to environment", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) CreateEnvironment(ctx context.Context, orgID string, name string, templateUUID string, prefix string) (*caliri.EnvironmentDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	envId := GetEnvironmentID(templateUUID)
	env, httpResp, err := client.OwnerAPI.CreateEnvironment(ctx, OwnerKey(orgID)).EnvironmentDTO(caliri.EnvironmentDTO{Id: &envId, Name: &name, ContentPrefix: &prefix, Type: pointy.Pointer(ENVIRONMENT_TYPE)}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("couldn't create environment", httpResp, err)
	}
	return env, nil
}

func (c *cpClientImpl) FetchEnvironment(ctx context.Context, templateID string) (*caliri.EnvironmentDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	resp, httpResp, err := client.EnvironmentAPI.GetEnvironment(ctx, GetEnvironmentID(templateID)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp.StatusCode == 404 {
			return nil, nil
		}
		return nil, errorWithResponseBody("couldn't fetch environment", httpResp, err)
	}
	return resp, nil
}

func (c *cpClientImpl) PromoteContentToEnvironment(ctx context.Context, templateID string, repoConfigUUIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	contentIDs := []string{}
	for _, repoUUID := range repoConfigUUIDs {
		contentIDs = append(contentIDs, GetContentID(repoUUID))
	}

	var contentToPromote []caliri.ContentToPromoteDTO
	for _, id := range contentIDs {
		contentID := id
		contentToPromote = append(contentToPromote, caliri.ContentToPromoteDTO{
			EnvironmentId: pointy.Pointer(GetEnvironmentID(templateID)),
			ContentId:     &contentID,
			Enabled:       pointy.Pointer(true),
		})
	}
	_, httpResp, err := client.EnvironmentAPI.PromoteContent(ctx, GetEnvironmentID(templateID)).ContentToPromoteDTO(contentToPromote).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't promote content to environment", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) DemoteContentFromEnvironment(ctx context.Context, templateID string, repoConfigUUIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	contentIDs := []string{}
	for _, repoUUID := range repoConfigUUIDs {
		contentIDs = append(contentIDs, GetContentID(repoUUID))
	}

	_, httpResp, err := client.EnvironmentAPI.DemoteContent(ctx, GetEnvironmentID(templateID)).Content(contentIDs).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't promote content to environment", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) UpdateContentOverrides(ctx context.Context, templateID string, dtos []caliri.ContentOverrideDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	_, httpResp, err := client.EnvironmentAPI.PutEnvironmentContentOverrides(ctx, GetEnvironmentID(templateID)).ContentOverrideDTO(dtos).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("could not override environment contents", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) RemoveContentOverrides(ctx context.Context, templateUUID string, toRemove []caliri.ContentOverrideDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	_, httpResp, err := client.EnvironmentAPI.DeleteEnvironmentContentOverrides(ctx, GetEnvironmentID(templateUUID)).ContentOverrideDTO(toRemove).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("could not remove overrides", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) FetchContentOverrides(ctx context.Context, templateUUID string) ([]caliri.ContentOverrideDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return []caliri.ContentOverrideDTO{}, err
	}

	overrides, httpResp, err := client.EnvironmentAPI.GetEnvironmentContentOverrides(ctx, GetEnvironmentID(templateUUID)).Execute()

	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return []caliri.ContentOverrideDTO{}, errorWithResponseBody("could not fetch environment contents", httpResp, err)
	}
	return overrides, nil
}

// FetchContentOverridesForRepo Behaves just like FetchContentOverrides but returns a subset of overrides only for a given repo
func (c *cpClientImpl) FetchContentOverridesForRepo(ctx context.Context, templateUUID string, label string) ([]caliri.ContentOverrideDTO, error) {
	overrides, err := c.FetchContentOverrides(ctx, GetEnvironmentID(templateUUID))
	if err != nil {
		return []caliri.ContentOverrideDTO{}, err
	}
	subset := []caliri.ContentOverrideDTO{}
	for _, override := range overrides {
		if *override.ContentLabel == label {
			subset = append(subset, override)
		}
	}
	return subset, nil
}

func (c *cpClientImpl) DeleteEnvironment(ctx context.Context, templateUUID string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	httpResp, err := client.EnvironmentAPI.DeleteEnvironment(ctx, GetEnvironmentID(templateUUID)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp.StatusCode == 404 {
			return nil
		}
		return errorWithResponseBody("couldn't delete environment", httpResp, err)
	}
	return nil
}
