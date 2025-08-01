import { expect, test } from 'test-utils';
import {
  PopularRepositoriesApi,
  RepositoriesApi,
  RpmsApi,
  ApiSearchRpmResponse,
  GetRepositoryRequest,
  ListRepositoriesRequest,
} from 'test-utils/client';

test.describe('Popular repositories', () => {
  test('List popular repositories', async ({ client }) => {
    const resp = await new PopularRepositoriesApi(client).listPopularRepositories({
      search: 'EPEL 9',
    });
    expect(resp.meta?.count).toBe(1);
    expect(resp.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
  });

  test('Test popular repos pagination api with limit and offset', async ({ client }) => {
    await test.step('Test limit parameter - get first repository only', async () => {
      const firstRepo = await new PopularRepositoriesApi(client).listPopularRepositories({
        limit: 1,
      });

      expect(firstRepo.data?.length).toBe(1);
      expect(firstRepo.meta?.count).toBe(3); // Total count should still be 3
      expect(firstRepo.data?.[0].suggestedName).toBe('EPEL 10 Everything x86_64');
    });

    await test.step('Test offset parameter - skip first repository', async () => {
      const secondRepo = await new PopularRepositoriesApi(client).listPopularRepositories({
        offset: 1,
      });

      expect(secondRepo.data?.length).toBe(2); // Should get repos 2 and 3
      expect(secondRepo.meta?.count).toBe(3); // Total count should still be 3
      expect(secondRepo.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
      expect(secondRepo.data?.[1].suggestedName).toBe('EPEL 8 Everything x86_64');
    });

    await test.step('Test limit + offset combination - get second repository only', async () => {
      const secondRepoOnly = await new PopularRepositoriesApi(client).listPopularRepositories({
        limit: 1,
        offset: 1,
      });

      expect(secondRepoOnly.data).toBeDefined();
      expect(secondRepoOnly.data?.length).toBe(1);
      expect(secondRepoOnly.meta?.count).toBe(3); // Total count should still be 3
      expect(secondRepoOnly.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
    });

    await test.step('Test edge case - offset beyond available data', async () => {
      const beyondData = await new PopularRepositoriesApi(client).listPopularRepositories({
        offset: 5, // Beyond the 3 available repos
      });

      expect(beyondData.data).toBeDefined();
      expect(beyondData.data?.length).toBe(0); // Should return empty array
      expect(beyondData.meta?.count).toBe(3); // Total count should still be 3
    });
  });

  test('Test that searching rpms in non-added popular repo does not return an empty list', async ({
    client,
  }) => {
    const epelUrl = 'https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/';
    let rpmSearch: ApiSearchRpmResponse[];

    await test.step('Search for RPMs in the EPEL repository URL', async () => {
      rpmSearch = await new RpmsApi(client).searchRpm({
        apiContentUnitSearchRequest: {
          search: 'epel-release',
          urls: [epelUrl],
        },
      });

      // Verify that search results are not empty
      expect(rpmSearch.length).toBeGreaterThan(0);

      // Verify that the first result is exactly "epel-release"
      expect(rpmSearch[0].packageName).toBe('epel-release');
    });
  });
});
