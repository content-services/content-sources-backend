import { expect } from '@playwright/test';

import { test } from './base_client';
import {
  ApiRepositoryResponse,
  DeleteTemplateRequest,
  GetRepositoryRequest,
  PartialUpdateTemplateRequest,
  RepositoriesApi,
  TemplatesApi,
} from './client';
import { randomName, randomUrl } from './helpers/repoHelpers';
import { cleanupRepositories, cleanupTemplates, poll } from './helpers/apiHelpers';

test('TemplateCRUD', async ({ client, cleanup }) => {
  const repoPrefix = 'Test-repo-for-template-CRUD';
  const repoUrl = randomUrl();
  const templatePrefix = 'Test-template-CRUD';

  await cleanup.runAndAdd(() => cleanupRepositories(client, repoPrefix, repoUrl));
  await cleanup.runAndAdd(() => cleanupTemplates(client, templatePrefix));

  const repo_uuid = await test.step('Create test repo', async () => {
    const repo_name = `${repoPrefix}-${randomName()}`;
    const repo = await new RepositoriesApi(client).createRepository({
      apiRepositoryRequest: { name: `${repo_name}`, snapshot: true, url: `${repoUrl}` },
    });
    expect(repo.name).toContain(repoPrefix);
    const repo_uuid = repo.uuid;
    return repo_uuid;
  });

  await test.step('Wait for introspection to be completed', async () => {
    const getRepository = () =>
      new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo_uuid?.toString(),
      });
    const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
    const resp = await poll(getRepository, waitWhilePending, 10);
    expect(resp.status).toBe('Valid');
  });

  const template_uuid = await test.step('Create a Template', async () => {
    const template_name = `${templatePrefix}-${randomName()}`;
    const template = await new TemplatesApi(client).createTemplate({
      apiTemplateRequest: {
        name: `${template_name}`,
        arch: 'x86_64',
        repositoryUuids: [`${repo_uuid}`],
        version: '9',
        description: 'Created the template',
      },
    });
    expect(template.name).toContain(templatePrefix);
    const template_uuid = template.uuid;
    return template_uuid;
  });

  await test.step('Read a Template', async () => {
    const resp = await new TemplatesApi(client).getTemplate({ uuid: `${template_uuid}` });
    expect(resp.uuid).toBe(template_uuid);
  });

  await test.step('Update the template', async () => {
    const resp = await new TemplatesApi(client).partialUpdateTemplate(<
      PartialUpdateTemplateRequest
    >{ uuid: template_uuid, apiTemplateUpdateRequest: { description: 'Updated the template' } });
    expect(resp.description).toBe('Updated the template');
  });

  await test.step('Delete a Template', async () => {
    const delResp = await new TemplatesApi(client).deleteTemplateRaw(<DeleteTemplateRequest>{
      uuid: `${template_uuid}`,
    });
    expect(delResp.raw.status).toBe(204);
  });
});
