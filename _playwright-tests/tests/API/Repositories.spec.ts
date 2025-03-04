import { test } from './base_client';
import {
  RepositoriesApi,
  GetRepositoryRequest,
  ApiRepositoryResponse,
  type ListRepositoriesRequest,
} from './client';
import { expect } from '@playwright/test';
import { poll } from './apiHelpers';

test.describe('Repositories', () => {
  test('Verify repository introspection', async ({ client }) => {
    await test.step('delete existing repository if exists', async () => {
      const existing = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
        search: 'test-repository',
      });

      if (existing?.data?.length) {
        const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
          uuid: existing.data[0].uuid?.toString(),
        });
        expect(resp.raw.status).toBe(204);
      }
    });

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: 'test-repository',
          url: 'https://content-services.github.io/fixtures/yum/comps-modules/v1/',
        },
      });
      expect(repo.name).toBe('test-repository');
    });

    await test.step('Wait for introspection to be completed', async () => {
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
});
