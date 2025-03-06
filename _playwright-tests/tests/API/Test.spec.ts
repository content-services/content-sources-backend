import { TestConfig, confToAnnotations, test } from './base_client';
import { RepositoriesApi, GetRepositoryRequest, ApiRepositoryResponse } from './client';
import { expect } from '@playwright/test';
import { cleanupRepositories, poll } from './apiHelpers';

test.describe('Test', () => {
  // --------------- Approach no. 1 ---------------
  const testConf: TestConfig = {
    repoNames: ['test-2', 'reponame'],
    urls: ['https://fedorapeople.org/groups/katello/fakerepos/zoo/'],
    // templatesNames: [...]
  };
  test('Test01', { annotation: [...confToAnnotations(testConf)] }, async ({ client }) => {
    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: testConf.repoNames?.at(0) ?? '',
          url: testConf.urls?.at(0),
        },
      });
      expect(repo.name).toBe(testConf.repoNames?.at(0));
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

  // --------------- Approach no. 2 ---------------
  test('Test02', async ({ client }) => {
    const names = ['test-2', 'reponame'];
    const url = 'https://fedorapeople.org/groups/katello/fakerepos/zoo/';

    await test.step(
      'pre-cleanup',
      async () => {
        await cleanupRepositories(client, names, [url]);
        // cleanupTemplates(client, [...])
      },
      { box: true },
    );

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
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

    await test.step(
      'post-cleanup',
      async () => {
        await cleanupRepositories(client, names, [url]);
        // cleanupTemplates(client, [...])
      },
      { box: true },
    );
  });
});
