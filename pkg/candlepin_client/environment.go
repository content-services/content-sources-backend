package candlepin_client

import (
	"context"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/openlyinc/pointy"
)

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

func (c *cpClientImpl) CreateEnvironment(ctx context.Context, ownerKey string, name string, id string, prefix string) (*caliri.EnvironmentDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	env, httpResp, err := client.OwnerAPI.CreateEnv(ctx, ownerKey).EnvironmentDTO(caliri.EnvironmentDTO{Id: &id, Name: &name, ContentPrefix: &prefix}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("couldn't create environment", httpResp, err)
	}
	return env, nil
}

func (c *cpClientImpl) FetchEnvironment(ctx context.Context, envID string) (*caliri.EnvironmentDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	resp, httpResp, err := client.EnvironmentAPI.GetEnvironment(ctx, envID).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("couldn't fetch environment", httpResp, err)
	}
	return resp, nil
}

func (c *cpClientImpl) PromoteContentToEnvironment(ctx context.Context, envID string, contentIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	var contentToPromote []caliri.ContentToPromoteDTO
	for _, id := range contentIDs {
		contentID := id
		contentToPromote = append(contentToPromote, caliri.ContentToPromoteDTO{
			EnvironmentId: &envID,
			ContentId:     &contentID,
			Enabled:       pointy.Pointer(true),
		})
	}
	_, httpResp, err := client.EnvironmentAPI.PromoteContent(ctx, envID).ContentToPromoteDTO(contentToPromote).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't promote content to environment", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) DemoteContentFromEnvironment(ctx context.Context, envID string, contentIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	_, httpResp, err := client.EnvironmentAPI.DemoteContent(ctx, envID).Content(contentIDs).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't promote content to environment", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) UpdateContentOverrides(ctx context.Context, environmentId string, dtos []caliri.ContentOverrideDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	_, httpResp, err := client.EnvironmentAPI.PutEnvironmentContentOverrides(ctx, environmentId).ContentOverrideDTO(dtos).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("could not override environment contents", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) RemoveContentOverrides(ctx context.Context, environmentId string, toRemove []caliri.ContentOverrideDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	_, httpResp, err := client.EnvironmentAPI.DeleteEnvironmentContentOverrides(ctx, environmentId).ContentOverrideDTO(toRemove).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("could not remove overrides", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) FetchContentPathOverrides(ctx context.Context, environmentId string) ([]caliri.ContentOverrideDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return []caliri.ContentOverrideDTO{}, err
	}

	overrides, httpResp, err := client.EnvironmentAPI.GetEnvironmentContentOverrides(ctx, environmentId).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return []caliri.ContentOverrideDTO{}, errorWithResponseBody("could not fetch environment contents", httpResp, err)
	}
	return overrides, nil
}

func (c *cpClientImpl) DeleteEnvironment(ctx context.Context, envID string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	httpResp, err := client.EnvironmentAPI.DeleteEnvironment(ctx, envID).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't delete environment", httpResp, err)
	}
	return nil
}
