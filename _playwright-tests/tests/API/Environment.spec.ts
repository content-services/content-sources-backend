import { test, expect, RepositoriesApi, EnvironmentsApi, ApiRepositoryResponse } from 'test-utils';
import { poll } from 'test-utils/helpers';

/**
 * Test the API support for search environments using hardcoded EPEL repository
 * This test validates that environment search endpoints work correctly for both URL and UUID searches.
 */

test.describe('Environment', () => {
  const testRepoUrl = 'https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/';
  const keyword = 'kde';

  test('Environment search with EPEL repository', async ({ client }) => {
    const repositoriesApi = new RepositoriesApi(client);
    const environmentsApi = new EnvironmentsApi(client);

    let testRepo: ApiRepositoryResponse;

    await test.step('Find existing EPEL repository', async () => {
      const existingRepos = await repositoriesApi.listRepositories({
        url: testRepoUrl,
        origin: 'community',
      });

      expect(existingRepos.data).toBeDefined();
      expect(existingRepos.data!.length).toBeGreaterThan(0);
      testRepo = existingRepos.data![0];

      // Wait for repository to finish introspection if it's pending
      const getRepository = () =>
        repositoriesApi.getRepository({
          uuid: testRepo.uuid!,
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
      expect(resp.uuid).toBeDefined();
      testRepo = resp;
    });

    await test.step('Search environments with keyword using repository URL', async () => {
      const environmentSearch = await environmentsApi.searchEnvironments({
        apiContentUnitSearchRequest: {
          urls: [testRepoUrl],
          search: keyword,
        },
      });

      expect(environmentSearch).toBeDefined();
      expect(environmentSearch.length).toBeGreaterThan(0);
      expect(environmentSearch[0].id).toContain(keyword);
    });

    await test.step('Search environments with keyword using repository UUID', async () => {
      const environmentSearchByUuid = await environmentsApi.searchEnvironments({
        apiContentUnitSearchRequest: {
          uuids: [testRepo.uuid!],
          search: keyword,
        },
      });

      expect(environmentSearchByUuid).toBeDefined();
      expect(environmentSearchByUuid.length).toBeGreaterThan(0);
      expect(environmentSearchByUuid[0].id).toContain(keyword);
      expect(environmentSearchByUuid[0].environmentName).toBeDefined();
    });
  });
});
