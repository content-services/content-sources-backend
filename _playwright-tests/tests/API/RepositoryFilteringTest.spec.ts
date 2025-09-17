import { expect, test } from 'test-utils';
import { RepositoriesApi, ApiRepositoryResponse, GetRepositoryRequest } from 'test-utils/client';
import { poll, randomName, cleanupRepositories } from 'test-utils/helpers';

/**
 * Tests that repository listings can be filtered by various parameters including
 * architecture, version, UUID, URL, text search and status.
 */

test.describe('Repository Filtering Test', () => {
  const createdRepos: ApiRepositoryResponse[] = [];

  test('should filter repository listings with various parameters including status', async ({
    client,
    cleanup,
  }) => {
    const baseRepoName = `Filter-Test-${randomName()}`;
    const repositoriesApi = new RepositoriesApi(client);

    const getRepoNames = (response: { data?: { name?: string }[] }) =>
      response.data?.map((repo) => repo.name) || [];

    await cleanup.runAndAdd(() =>
      cleanupRepositories(
        client,
        baseRepoName,
        'https://dl.fedoraproject.org/pub/epel/8/Everything/ppc64le/',
        'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/',
        'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/',
        'https://non-existent-domain-for-testing-invalid-status.invalid/repo/',
      ),
    );

    await test.step('create test repositories', async () => {
      const repo1 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-1`,
          url: 'https://dl.fedoraproject.org/pub/epel/8/Everything/ppc64le/',
          distributionArch: 'ppc64le',
          distributionVersions: ['8'],
        },
      });
      createdRepos.push(repo1);

      const repo2 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-2`,
          url: 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/',
          distributionArch: 'x86_64',
          distributionVersions: ['9'],
        },
      });
      createdRepos.push(repo2);

      const repo3 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-3`,
          url: 'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/',
          distributionArch: 'x86_64',
          distributionVersions: ['8'],
        },
      });
      createdRepos.push(repo3);

      expect(createdRepos).toHaveLength(3);
    });

    await test.step('Wait for repositories to be introspected', async () => {
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';

      for (const repo of createdRepos) {
        const getRepository = () =>
          repositoriesApi.getRepository(<GetRepositoryRequest>{
            uuid: repo.uuid?.toString(),
          });

        const updatedRepo = await poll(getRepository, waitWhilePending, 10);
        expect(updatedRepo.status).toBe('Valid');
      }
    });

    await test.step('Test single-value filters', async () => {
      const x86Response = await repositoriesApi.listRepositories({
        origin: 'external',
        arch: 'x86_64',
      });

      const x86RepoNames = getRepoNames(x86Response);
      expect(x86RepoNames).toContain(`${baseRepoName}-2`);
      expect(x86RepoNames).toContain(`${baseRepoName}-3`);
      expect(x86RepoNames).not.toContain(`${baseRepoName}-1`);

      const version8Response = await repositoriesApi.listRepositories({
        origin: 'external',
        version: '8',
      });

      const version8RepoNames = getRepoNames(version8Response);
      expect(version8RepoNames).toContain(`${baseRepoName}-1`);
      expect(version8RepoNames).toContain(`${baseRepoName}-3`);
      expect(version8RepoNames).not.toContain(`${baseRepoName}-2`);

      const availableVersion9Response = await new RepositoriesApi(client).listRepositories({
        availableForVersion: '9',
        origin: 'external',
      });

      const availableVersion9Names = availableVersion9Response.data?.map((repo) => repo.name) || [];
      expect(availableVersion9Names).toContain(`${baseRepoName}-2`);

      const availablePpc64Response = await new RepositoriesApi(client).listRepositories({
        availableForArch: 'ppc64le',
        origin: 'external',
      });

      const availablePpc64Names = availablePpc64Response.data?.map((repo) => repo.name) || [];
      expect(availablePpc64Names).toContain(`${baseRepoName}-1`);
    });

    await test.step('Test multi-value filters (comma-separated)', async () => {
      const multiArchResponse = await new RepositoriesApi(client).listRepositories({
        arch: 'x86_64,ppc64le',
        origin: 'external',
      });

      const multiArchNames = multiArchResponse.data?.map((repo) => repo.name) || [];
      expect(multiArchNames).toContain(`${baseRepoName}-1`);
      expect(multiArchNames).toContain(`${baseRepoName}-2`);
      expect(multiArchNames).toContain(`${baseRepoName}-3`);

      const multiVersionResponse = await new RepositoriesApi(client).listRepositories({
        version: '8,9',
        origin: 'external',
      });

      const multiVersionNames = multiVersionResponse.data?.map((repo) => repo.name) || [];
      expect(multiVersionNames).toContain(`${baseRepoName}-1`);
      expect(multiVersionNames).toContain(`${baseRepoName}-2`);
      expect(multiVersionNames).toContain(`${baseRepoName}-3`);
    });

    await test.step('Test UUID-based filtering', async () => {
      const repo1 = createdRepos[0];
      const repo2 = createdRepos[1];

      const singleUuidResponse = await new RepositoriesApi(client).listRepositories({
        uuid: repo1.uuid,
        origin: 'external',
      });

      expect(singleUuidResponse.meta?.count).toBe(1);
      expect(singleUuidResponse.data?.[0]?.uuid).toBe(repo1.uuid);

      const multiUuidResponse = await new RepositoriesApi(client).listRepositories({
        uuid: `${repo1.uuid},${repo2.uuid}`,
        origin: 'external',
      });

      expect(multiUuidResponse.meta?.count).toBe(2);
      const returnedUuids = multiUuidResponse.data?.map((repo) => repo.uuid) || [];
      expect(returnedUuids).toContain(repo1.uuid);
      expect(returnedUuids).toContain(repo2.uuid);
    });

    await test.step('Test URL-based filtering', async () => {
      const repo1 = createdRepos[0];
      const repo2 = createdRepos[1];

      const singleUrlResponse = await new RepositoriesApi(client).listRepositories({
        url: repo1.url,
        origin: 'external',
      });

      expect(singleUrlResponse.meta?.count).toBe(1);
      expect(singleUrlResponse.data?.[0]?.url).toBe(repo1.url);

      const multiUrlResponse = await new RepositoriesApi(client).listRepositories({
        url: `${repo1.url},${repo2.url}`,
        origin: 'external',
      });

      expect(multiUrlResponse.meta?.count).toBe(2);
      const returnedUrls = multiUrlResponse.data?.map((repo) => repo.url) || [];
      expect(returnedUrls).toContain(repo1.url);
      expect(returnedUrls).toContain(repo2.url);
    });

    await test.step('Test text search functionality', async () => {
      const repo1 = createdRepos[0];

      const nameSearchResponse = await new RepositoriesApi(client).listRepositories({
        search: repo1.name,
        origin: 'external',
      });

      expect(nameSearchResponse.meta?.count).toBe(1);
      expect(nameSearchResponse.data?.[0]?.name).toBe(repo1.name);

      const partialSearchResponse = await new RepositoriesApi(client).listRepositories({
        search: baseRepoName,
        origin: 'external',
      });

      expect(partialSearchResponse.meta?.count).toBeGreaterThanOrEqual(3);
      const searchResultNames = partialSearchResponse.data?.map((repo) => repo.name) || [];
      expect(searchResultNames).toContain(`${baseRepoName}-1`);
      expect(searchResultNames).toContain(`${baseRepoName}-2`);
      expect(searchResultNames).toContain(`${baseRepoName}-3`);
    });

    await test.step('Test status-based filtering', async () => {
      const invalidRepoUrl = 'https://non-existent-domain-for-testing-invalid-status.invalid/repo/';

      const invalidRepo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-invalid`,
          url: invalidRepoUrl,
          distributionArch: 's390x',
          distributionVersions: ['8'],
        },
      });

      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';

      const getInvalidRepo = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: invalidRepo.uuid?.toString(),
        });

      const invalidRepoResult = await poll(getInvalidRepo, waitWhilePending, 10);
      expect(invalidRepoResult.status).toBe('Invalid');

      const pendingValidResponse = await new RepositoriesApi(client).listRepositories({
        status: 'Pending,Valid',
        origin: 'external',
      });

      const validStatusNames = pendingValidResponse.data?.map((repo) => repo.name) || [];
      const validStatuses = pendingValidResponse.data?.map((repo) => repo.status) || [];

      expect(validStatusNames).toContain(`${baseRepoName}-1`);
      expect(validStatusNames).toContain(`${baseRepoName}-2`);
      expect(validStatusNames).toContain(`${baseRepoName}-3`);
      expect(validStatusNames).not.toContain(`${baseRepoName}-invalid`);
      expect(validStatuses).not.toContain('Invalid');

      const invalidResponse = await new RepositoriesApi(client).listRepositories({
        status: 'Invalid',
        origin: 'external',
      });

      const invalidStatusNames = invalidResponse.data?.map((repo) => repo.name) || [];
      const invalidStatuses = invalidResponse.data?.map((repo) => repo.status) || [];

      expect(invalidStatusNames).toContain(`${baseRepoName}-invalid`);
      expect(invalidStatusNames).not.toContain(`${baseRepoName}-1`);
      expect(invalidStatusNames).not.toContain(`${baseRepoName}-2`);
      expect(invalidStatusNames).not.toContain(`${baseRepoName}-3`);
      invalidStatuses.forEach((status) => {
        expect(status).toBe('Invalid');
      });
    });
  });
});
