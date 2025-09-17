import { expect, test } from 'test-utils';
import { ApiRepositoryResponse, GetRepositoryRequest, RepositoriesApi } from 'test-utils/client';
import { poll, randomName, cleanupRepositories } from 'test-utils/helpers';

/**
 * Tests that repository listings can be filtered by various parameters including
 * architecture, version, UUID, URL, text search and status.
 */

test.describe('Repository Filtering Test', () => {
  test('Verify repository filtering', async ({ client, cleanup }) => {
    const baseRepoName = `Filter-Test-${randomName()}`;
    const repositoriesApi = new RepositoriesApi(client);
    const createdRepos: ApiRepositoryResponse[] = [];

    await cleanup.runAndAdd(() =>
      cleanupRepositories(
        client,
        baseRepoName,
        'https://content-services.github.io/fixtures/yum/centirepos/repo02/',
        'https://content-services.github.io/fixtures/yum/centirepos/repo03/',
        'https://content-services.github.io/fixtures/yum/centirepos/repo04/',
        'https://non-existent-domain-for-testing-invalid-status.invalid/repo/',
      ),
    );

    await test.step('Create test repositories', async () => {
      const repo1 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-1`,
          url: 'https://content-services.github.io/fixtures/yum/centirepos/repo02/',
          distributionArch: 'ppc64le',
          distributionVersions: ['8'],
        },
      });
      createdRepos.push(repo1);

      const repo2 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-2`,
          url: 'https://content-services.github.io/fixtures/yum/centirepos/repo03/',
          distributionArch: 'x86_64',
          distributionVersions: ['9'],
        },
      });
      createdRepos.push(repo2);

      const repo3 = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-3`,
          url: 'https://content-services.github.io/fixtures/yum/centirepos/repo04/',
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
        search: baseRepoName,
      });

      const x86RepoNames = x86Response.data?.map((repo) => repo.name) || [];
      expect(x86Response.meta?.count).toBe(2);
      x86Response.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.distributionArch).toBe('x86_64');
      });
      expect(x86RepoNames).toContain(`${baseRepoName}-2`);
      expect(x86RepoNames).toContain(`${baseRepoName}-3`);
      expect(x86RepoNames).not.toContain(`${baseRepoName}-1`);

      const version8Response = await repositoriesApi.listRepositories({
        origin: 'external',
        version: '8',
        search: baseRepoName,
      });

      const version8RepoNames = version8Response.data?.map((repo) => repo.name) || [];
      expect(version8Response.meta?.count).toBe(2);
      version8Response.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.distributionVersions).toContain('8');
      });
      expect(version8RepoNames).toContain(`${baseRepoName}-1`);
      expect(version8RepoNames).toContain(`${baseRepoName}-3`);
      expect(version8RepoNames).not.toContain(`${baseRepoName}-2`);

      const availableVersion9Response = await repositoriesApi.listRepositories({
        availableForVersion: '9',
        origin: 'external',
        search: baseRepoName,
      });

      const availableVersion9Names = availableVersion9Response.data?.map((repo) => repo.name) || [];
      expect(availableVersion9Response.meta?.count).toBe(1);
      availableVersion9Response.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.distributionVersions).toContain('9');
      });
      expect(availableVersion9Names).toContain(`${baseRepoName}-2`);

      const availablePpc64Response = await repositoriesApi.listRepositories({
        availableForArch: 'ppc64le',
        origin: 'external',
        search: baseRepoName,
      });

      const availablePpc64Names = availablePpc64Response.data?.map((repo) => repo.name) || [];
      expect(availablePpc64Response.meta?.count).toBe(1);
      availablePpc64Response.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.distributionArch).toBe('ppc64le');
      });
      expect(availablePpc64Names).toContain(`${baseRepoName}-1`);
    });

    await test.step('Test multi-value filters (comma-separated)', async () => {
      const multiArchResponse = await repositoriesApi.listRepositories({
        arch: 'x86_64,ppc64le',
        origin: 'external',
        search: baseRepoName,
      });

      const multiArchNames = multiArchResponse.data?.map((repo) => repo.name) || [];
      expect(multiArchResponse.meta?.count).toBe(3);
      multiArchResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(['x86_64', 'ppc64le']).toContain(repo.distributionArch);
      });
      expect(multiArchNames).toContain(`${baseRepoName}-1`);
      expect(multiArchNames).toContain(`${baseRepoName}-2`);
      expect(multiArchNames).toContain(`${baseRepoName}-3`);

      const multiVersionResponse = await repositoriesApi.listRepositories({
        version: '8,9',
        origin: 'external',
        search: baseRepoName,
      });

      const multiVersionNames = multiVersionResponse.data?.map((repo) => repo.name) || [];
      expect(multiVersionResponse.meta?.count).toBe(3);
      multiVersionResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        const hasVersion8 = repo.distributionVersions?.includes('8');
        const hasVersion9 = repo.distributionVersions?.includes('9');
        expect(hasVersion8 || hasVersion9).toBeTruthy();
      });
      expect(multiVersionNames).toContain(`${baseRepoName}-1`);
      expect(multiVersionNames).toContain(`${baseRepoName}-2`);
      expect(multiVersionNames).toContain(`${baseRepoName}-3`);
    });

    await test.step('Test UUID-based filtering', async () => {
      const repo1 = createdRepos[0];
      const repo2 = createdRepos[1];

      const singleUuidResponse = await repositoriesApi.listRepositories({
        uuid: repo1.uuid,
        origin: 'external',
      });

      expect(singleUuidResponse.meta?.count).toBe(1);
      expect(singleUuidResponse.data?.[0]?.uuid).toBe(repo1.uuid);

      const multiUuidResponse = await repositoriesApi.listRepositories({
        uuid: `${repo1.uuid},${repo2.uuid}`,
        origin: 'external',
      });

      expect(multiUuidResponse.meta?.count).toBe(2);
      const returnedUuids = multiUuidResponse.data?.map((repo) => repo.uuid) || [];
      multiUuidResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect([repo1.uuid, repo2.uuid]).toContain(repo.uuid);
      });
      expect(returnedUuids).toContain(repo1.uuid);
      expect(returnedUuids).toContain(repo2.uuid);
    });

    await test.step('Test URL-based filtering', async () => {
      const repo1 = createdRepos[0];
      const repo2 = createdRepos[1];

      const singleUrlResponse = await repositoriesApi.listRepositories({
        url: repo1.url,
        origin: 'external',
      });

      expect(singleUrlResponse.meta?.count).toBe(1);
      expect(singleUrlResponse.data?.[0]?.url).toBe(repo1.url);

      const multiUrlResponse = await repositoriesApi.listRepositories({
        url: `${repo1.url},${repo2.url}`,
        origin: 'external',
      });

      expect(multiUrlResponse.meta?.count).toBe(2);
      const returnedUrls = multiUrlResponse.data?.map((repo) => repo.url) || [];
      multiUrlResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect([repo1.url, repo2.url]).toContain(repo.url);
      });
      expect(returnedUrls).toContain(repo1.url);
      expect(returnedUrls).toContain(repo2.url);
    });

    await test.step('Test text search functionality', async () => {
      const repo1 = createdRepos[0];

      const nameSearchResponse = await repositoriesApi.listRepositories({
        search: repo1.name,
        origin: 'external',
      });

      expect(nameSearchResponse.meta?.count).toBe(1);
      expect(nameSearchResponse.data?.[0]?.name).toBe(repo1.name);

      const partialSearchResponse = await repositoriesApi.listRepositories({
        search: baseRepoName,
        origin: 'external',
      });

      expect(partialSearchResponse.meta?.count).toBeGreaterThanOrEqual(3);
      expect(partialSearchResponse.data?.length).toBe(partialSearchResponse.meta?.count);
      const searchResultNames = partialSearchResponse.data?.map((repo) => repo.name) || [];
      partialSearchResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.name?.toLowerCase()).toContain(baseRepoName.toLowerCase());
      });
      expect(searchResultNames).toContain(`${baseRepoName}-1`);
      expect(searchResultNames).toContain(`${baseRepoName}-2`);
      expect(searchResultNames).toContain(`${baseRepoName}-3`);
    });

    await test.step('Test status-based filtering', async () => {
      const invalidRepoUrl = 'https://non-existent-domain-for-testing-invalid-status.invalid/repo/';

      const invalidRepo = await repositoriesApi.createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-invalid`,
          url: invalidRepoUrl,
          distributionArch: 's390x',
          distributionVersions: ['8'],
        },
      });

      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';

      const getInvalidRepo = () =>
        repositoriesApi.getRepository(<GetRepositoryRequest>{
          uuid: invalidRepo.uuid?.toString(),
        });

      const invalidRepoResult = await poll(getInvalidRepo, waitWhilePending, 10);
      expect(invalidRepoResult.status).toBe('Invalid');

      const pendingValidResponse = await repositoriesApi.listRepositories({
        status: 'Pending,Valid',
        origin: 'external',
        search: baseRepoName,
      });

      const validStatusNames = pendingValidResponse.data?.map((repo) => repo.name) || [];
      const validStatuses = pendingValidResponse.data?.map((repo) => repo.status) || [];

      expect(pendingValidResponse.meta?.count).toBe(3);
      pendingValidResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(['Pending', 'Valid']).toContain(repo.status);
      });
      expect(validStatusNames).toContain(`${baseRepoName}-1`);
      expect(validStatusNames).toContain(`${baseRepoName}-2`);
      expect(validStatusNames).toContain(`${baseRepoName}-3`);
      expect(validStatusNames).not.toContain(`${baseRepoName}-invalid`);
      expect(validStatuses).not.toContain('Invalid');

      const invalidResponse = await repositoriesApi.listRepositories({
        status: 'Invalid',
        origin: 'external',
        search: baseRepoName,
      });

      const invalidStatusNames = invalidResponse.data?.map((repo) => repo.name) || [];
      const invalidStatuses = invalidResponse.data?.map((repo) => repo.status) || [];

      expect(invalidResponse.meta?.count).toBe(1);
      invalidResponse.data?.forEach((repo: ApiRepositoryResponse) => {
        expect(repo.status).toBe('Invalid');
      });
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
