import { expect, test } from 'test-utils';
import { EnvironmentsApi, PackagegroupsApi, RepositoriesApi, RpmsApi } from 'test-utils/client';
import { cleanupRepositories, randomName, waitWhileRepositoryIsPending } from 'test-utils/helpers';

test.describe('Dated Names Search', () => {
  test('Verify "names search" functionality with and without a date', async ({
    client,
    cleanup,
  }) => {
    const firstURL = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';
    const secondURL = 'https://content-services.github.io/fixtures/yum/comps-modules/v2/';
    const repoName = randomName();
    let repoUUID: string;

    await cleanup.runAndAdd(() => cleanupRepositories(client, secondURL, repoName));

    await test.step('Create repo with 2 snapshots', async () => {
      const repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: firstURL,
          snapshot: true,
        },
      });

      expect(repo.uuid).toBeDefined();
      if (repo.uuid) repoUUID = repo.uuid;
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(firstURL);

      const resp = await waitWhileRepositoryIsPending(client, repoUUID);
      expect(resp.status).toBe('Valid');

      const updatedRepo = await new RepositoriesApi(client).partialUpdateRepository({
        uuid: repo.uuid!,
        apiRepositoryUpdateRequest: {
          url: secondURL,
        },
      });

      expect(updatedRepo.uuid).toBe(repo.uuid);
      expect(updatedRepo.name).toBe(repoName);
      expect(updatedRepo.url).toBe(secondURL);

      const updatedResp = await waitWhileRepositoryIsPending(client, repoUUID);
      expect(updatedResp.status).toBe('Valid');
    });

    await test.step('Search current names', async () => {
      const rpmSearch = await new RpmsApi(client).searchRpm({
        apiContentUnitSearchRequest: { uuids: [repoUUID], search: 'cat' },
      });
      expect(rpmSearch.length).toBe(1);
      const pkgNames = rpmSearch.map((v) => v.packageName);
      expect(pkgNames).toContain('cat');

      const packageGroups = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { uuids: [repoUUID], search: 's' },
      });
      expect(packageGroups.length).toBe(2);
      const pkgGroupNames = packageGroups.map((v) => v.packageGroupName);
      expect(pkgGroupNames).toContain('birds');
      expect(pkgGroupNames).toContain('mammals');

      const environments = await new EnvironmentsApi(client).searchEnvironments({
        apiContentUnitSearchRequest: { uuids: [repoUUID], search: 's' },
      });
      expect(environments.length).toBe(2);
      const envNames = environments.map((v) => v.environmentName);
      expect(envNames).toContain('avians');
      expect(envNames).toContain('animals');
    });

    await test.step('Search names for a date', async () => {
      const rpmSearch = await new RpmsApi(client).searchRpm({
        apiContentUnitSearchRequest: {
          uuids: [repoUUID],
          search: 'stork',
          date: '2025-08-24T00:00:00Z',
        },
      });
      expect(rpmSearch.length).toBe(1);
      const pkgNames = rpmSearch.map((v) => v.packageName);
      expect(pkgNames).toContain('stork');
      expect(pkgNames).not.toContain('cat');

      const packageGroups = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: {
          uuids: [repoUUID],
          search: 's',
          date: '2025-08-24T00:00:00Z',
        },
      });
      expect(packageGroups.length).toBe(1);
      const pkgGroupNames = packageGroups.map((v) => v.packageGroupName);
      expect(pkgGroupNames).toContain('birds');

      const environments = await new EnvironmentsApi(client).searchEnvironments({
        apiContentUnitSearchRequest: {
          uuids: [repoUUID],
          search: 's',
          date: '2025-08-24T00:00:00Z',
        },
      });
      expect(environments.length).toBe(1);
      const names = environments.map((v) => v.environmentName);
      expect(names).toContain('avians');
    });
  });
});
