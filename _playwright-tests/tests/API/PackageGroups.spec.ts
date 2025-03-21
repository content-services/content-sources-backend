import { expectError, test } from './base_client';
import {
  ApiRepositoryResponse,
  GetRepositoryRequest,
  PackagegroupsApi,
  RepositoriesApi,
} from './client';
import { expect } from '@playwright/test';
import { randomName } from './helpers/repoHelpers';
import { cleanupRepositories, poll } from './helpers/apiHelpers';
import { randomUUID } from 'crypto';

test.describe('Package groups', () => {
  test('Search and list package groups', async ({ client, cleanup }) => {
    const repoUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';
    const repoName = randomName();
    let repoUuid: string;
    const expectedGroup = 'birds';
    const expectedPackageList = ['penguin', 'duck', 'cockateel', 'stork'];

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoUrl, repoName));

    await test.step('Create repo with package groups', async () => {
      const repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
        },
      });

      expect(repo.uuid).toBeDefined();
      if (repo.uuid) {
        repoUuid = repo.uuid;
      }
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
    });

    await test.step('Wait for repo to be introspected', async () => {
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repoUuid,
        });
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
    });

    await test.step('Partial search for package groups', async () => {
      const partialMatchesWithUrl = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [repoUrl], uuids: [], search: 'b' },
      });

      const partialMatchesWithUuid = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [], uuids: [repoUuid], search: 'b' },
      });

      const noMatches = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [], uuids: [repoUuid], search: 'x' },
      });

      expect(partialMatchesWithUrl.length).toBe(1);
      expect(partialMatchesWithUrl[0].packageGroupName).toBe(expectedGroup);
      expect(partialMatchesWithUrl[0].packageList?.length).toBe(expectedPackageList.length);
      expect(partialMatchesWithUrl[0].packageList?.sort()).toStrictEqual(
        expectedPackageList.sort(),
      );
      expect(partialMatchesWithUuid.length).toBe(partialMatchesWithUrl.length);
      expect(partialMatchesWithUuid[0].packageGroupName).toBe(
        partialMatchesWithUrl[0].packageGroupName,
      );
      expect(partialMatchesWithUuid[0].packageList?.length).toBe(
        partialMatchesWithUrl[0].packageList?.length,
      );
      expect(partialMatchesWithUuid[0].packageList?.sort()).toStrictEqual(
        partialMatchesWithUrl[0].packageList?.sort(),
      );
      expect(noMatches.length).toBe(0);
    });

    await test.step('Exact search for package groups', async () => {
      const exactMatchesWithUrl = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [repoUrl], uuids: [], exactNames: [expectedGroup] },
      });

      const exactMatchesWithUuid = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [], uuids: [repoUuid], exactNames: [expectedGroup] },
      });

      const noMatches = await new PackagegroupsApi(client).searchPackageGroup({
        apiContentUnitSearchRequest: { urls: [], uuids: [repoUuid], exactNames: ['fake-group'] },
      });

      expect(exactMatchesWithUrl.length).toBe(1);
      expect(exactMatchesWithUrl[0].packageGroupName).toBe(expectedGroup);
      expect(exactMatchesWithUrl[0].packageList?.length).toBe(expectedPackageList.length);
      expect(exactMatchesWithUrl[0].packageList?.sort()).toStrictEqual(expectedPackageList.sort());
      expect(exactMatchesWithUuid.length).toBe(exactMatchesWithUrl.length);
      expect(exactMatchesWithUuid[0].packageGroupName).toBe(
        exactMatchesWithUrl[0].packageGroupName,
      );
      expect(exactMatchesWithUuid[0].packageList?.length).toBe(
        exactMatchesWithUrl[0].packageList?.length,
      );
      expect(exactMatchesWithUuid[0].packageList?.sort()).toStrictEqual(
        exactMatchesWithUrl[0].packageList?.sort(),
      );
      expect(noMatches.length).toBe(0);
    });

    await test.step('List package groups of a repo', async () => {
      const packageGroups = await new PackagegroupsApi(client).listRepositoriesPackageGroups({
        uuid: repoUuid,
      });

      expect(packageGroups?.data?.length).toBe(1);
      expect(packageGroups?.data?.[0].name).toBe(expectedGroup);
      expect(packageGroups?.data?.[0].packagelist?.sort()).toStrictEqual(
        expectedPackageList.sort(),
      );
    });

    await test.step('Searching package groups with a repo url or uuid that does not exist results in 404', async () => {
      const fakeUuid = randomUUID();
      const fakeUrl = 'https://fake-repo.com';

      await expectError(
        404,
        'Could not find repository with URL',
        new PackagegroupsApi(client).searchPackageGroup({
          apiContentUnitSearchRequest: {
            urls: [fakeUrl],
            uuids: [],
            exactNames: [expectedGroup],
          },
        }),
      );

      await expectError(
        404,
        'Could not find repository with UUID',
        new PackagegroupsApi(client).searchPackageGroup({
          apiContentUnitSearchRequest: {
            urls: [],
            uuids: [fakeUuid],
            exactNames: [expectedGroup],
          },
        }),
      );
    });

    await test.step('Listing package groups with a repo uuid that does not exist results in 404', async () => {
      const fakeUuid = randomUUID();

      await expectError(
        404,
        'Could not find repository with UUID',
        new PackagegroupsApi(client).listRepositoriesPackageGroups({
          uuid: fakeUuid,
        }),
      );
    });
  });
});
