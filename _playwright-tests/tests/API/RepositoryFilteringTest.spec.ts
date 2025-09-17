import { expect, test } from 'test-utils';
import { RepositoriesApi, ApiRepositoryResponse, GetRepositoryRequest } from 'test-utils/client';
import { poll, randomName, cleanupRepositories } from 'test-utils/helpers';

/**
 * Tests that repository listings can be filtered by various parameters including
 * architecture, version, UUID, URL, and text search.
 */

test.describe('Repository Filtering Test', () => {
  const createdRepos: ApiRepositoryResponse[] = [];

  test('should filter repository listings with various parameters', async ({ client, cleanup }) => {
    const baseRepoName = `Filter-Test-${randomName()}`;

    await cleanup.runAndAdd(() =>
      cleanupRepositories(
        client,
        baseRepoName,
        'https://dl.fedoraproject.org/pub/epel/8/Everything/ppc64le/',
        'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/',
        'https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/',
      ),
    );

    await test.step('create test repositories', async () => {
      const repo1 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-1`,
          url: 'https://dl.fedoraproject.org/pub/epel/8/Everything/ppc64le/',
          distributionArch: 'ppc64le',
          distributionVersions: ['8'],
        },
      });
      createdRepos.push(repo1);

      const repo2 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: `${baseRepoName}-2`,
          url: 'https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/',
          distributionArch: 'x86_64',
          distributionVersions: ['9'],
        },
      });
      createdRepos.push(repo2);

      const repo3 = await new RepositoriesApi(client).createRepository({
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
          new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
            uuid: repo.uuid?.toString(),
          });

        const updatedRepo = await poll(getRepository, waitWhilePending, 10);
        expect(updatedRepo.status).toBe('Valid');
      }
    });

    await test.step('Test single-value filters', async () => {
      const x86Response = await new RepositoriesApi(client).listRepositories({
        arch: 'x86_64',
        origin: 'external',
      });

      const x86RepoNames = x86Response.data?.map((repo) => repo.name) || [];
      expect(x86RepoNames).toContain(`${baseRepoName}-2`);
      expect(x86RepoNames).toContain(`${baseRepoName}-3`);
      expect(x86RepoNames).not.toContain(`${baseRepoName}-1`);

      const version8Response = await new RepositoriesApi(client).listRepositories({
        version: '8',
        origin: 'external',
      });

      const version8RepoNames = version8Response.data?.map((repo) => repo.name) || [];
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
  });
});
