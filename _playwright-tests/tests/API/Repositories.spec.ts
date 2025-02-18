import { test } from './base_client';
import { RepositoriesApi, GetRepositoryRequest, ApiRepositoryResponse } from './client';
import { expect } from '@playwright/test';
import { poll } from '../helpers/apiHelpers';

test('Content > Verify repository introspection', async ({ client }) => {
  const repo = await new RepositoriesApi(client).createRepository({
    apiRepositoryRequest: {
      name: 'test-repository',
      url: 'https://rverdile.fedorapeople.org/dummy-repos/modules/repo1/',
    },
  });
  expect(repo.name).toBe('test-repository');

  await test.step('wait for introspection to be completed', async () => {
    const getRepository = () =>
      new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid?.toString(),
      });
    const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
    const resp = await poll(getRepository, waitWhilePending, 10);
    expect(resp.status).toBe('Valid');
  });

  await test.step('delete repository', async () => {
    const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
      uuid: repo.uuid?.toString(),
    });
    expect(resp.raw.status).toBe(204);
  });
});
