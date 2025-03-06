import { test } from './base_client';
import { RepositoriesApi, GetRepositoryRequest, ApiRepositoryResponse } from './client';
import { expect } from '@playwright/test';
import { cleanupRepositories, poll } from './helpers/apiHelpers';

test.describe('Test', () => {
  test('Test', async ({ client, cleanup }) => {
    const names = ['test-2', 'reponame'];
    const url = 'https://fedorapeople.org/groups/katello/fakerepos/zoo/';

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
      await cleanup.runAndAdd(() => cleanupRepositories(client, names[0], url, names[1]));
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: names[0],
          url: url,
        },
      });
      expect(repo.name).toBe(names[0]);
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
  });
});
