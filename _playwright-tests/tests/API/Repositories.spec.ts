import { expect, test } from 'test-utils';
import {
  RepositoriesApi,
  GetRepositoryRequest,
  ListRepositoriesRequest,
  ApiRepositoryResponse,
  ApiRepositoryValidationResponseFromJSON,
  type ValidateRepositoryParametersRequest,
  RpmsApi,
  ApiSearchRpmResponse,
  CreateRepositoryRequest,
  ApiPopularRepositoriesCollectionResponse,
  PopularRepositoriesApi,
  TasksApi,
  CreateSnapshotRequest,
  GetTaskRequest,
  ApiTaskInfoResponse,
} from 'test-utils/client';
import {
  cleanupRepositories,
  poll,
  randomName,
  randomUrl,
  SmallRedHatRepoURL,
} from 'test-utils/helpers';

test.describe('Repositories', () => {
  test('Verify repository introspection', async ({ client, cleanup }) => {
    const repoName = `verify-repository-introspection-${randomName()}`;
    const repoUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, repoUrl));

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
        },
      });
      expect(repo.name).toBe(repoName);
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

  test('Validate repository parameters', async ({ client, cleanup }) => {
    const invalidFormatUuid = '49742069-edff-f58f-a2dd-5eb068444888';
    const repoName = `validate-repository-parameters-${randomName()}`;
    const repoUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v2/';
    const realButBadRepoUrl = 'http://jlsherrill.fedorapeople.org/fake-repos/';

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, repoUrl));

    await test.step('Check that a URLs protocol is supported and yum metadata can be retrieved', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            url: repoUrl,
          },
        ],
      });

      expect(resp[0].url?.httpCode).toBe(200);
      expect(resp[0].url?.metadataPresent).toBeTruthy();
    });

    await test.step('Check that lack of yum metadata is detected', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            url: realButBadRepoUrl,
          },
        ],
      });

      expect(resp[0].url?.httpCode).toBe(404);
      expect(resp[0].url?.metadataPresent).not.toBeTruthy();
    });

    await test.step('Check that a random name is valid for use', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            name: repoName,
          },
        ],
      });

      expect(resp[0].name?.valid).toBeTruthy();
    });

    let repo: ApiRepositoryResponse;
    await test.step('Create repo for unique checks', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
        },
      });

      expect(repo.name).toBe(repoName);
    });

    await test.step('Check that url and name has to be unique', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            name: repoName,
            url: repoUrl,
          },
        ],
      });

      expect(resp[0].name?.valid).not.toBeTruthy();
      expect(resp[0].name?.error).toContain(
        `A repository with the name '${repoName}' already exists.`,
      );
      expect(resp[0].url?.valid).not.toBeTruthy();
      expect(resp[0].url?.error).toContain(
        `A repository with the URL '${repoUrl}' already exists.`,
      );
    });

    await test.step('Repeat the check that url and name must be unique, but ignore repo with UUID given', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            name: repoName,
            url: repoUrl,
            uuid: repo.uuid,
          },
        ],
      });

      expect(resp[0].name?.valid).toBeTruthy();
      expect(resp[0].url?.valid).toBeTruthy();
    });

    await test.step('Check that invalid uuid does not cause an ISE', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParametersRaw(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            uuid: invalidFormatUuid,
          },
        ],
      });
      const json = ApiRepositoryValidationResponseFromJSON((await resp.raw.json())[0]);

      expect(resp.raw.status).toEqual(200);
      expect(json.name?.skipped).toBeTruthy();
      expect(json.url?.skipped).toBeTruthy();
      expect(json.url?.httpCode).toEqual(0);
    });

    await test.step('Check that invalid uuid does not cause an ISE, while validating other values', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParametersRaw(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            uuid: invalidFormatUuid,
            url: repoUrl,
            name: repoName,
          },
        ],
      });
      const json = ApiRepositoryValidationResponseFromJSON((await resp.raw.json())[0]);

      expect(resp.raw.status).toEqual(200);
      expect(json.name?.skipped).not.toBeTruthy();
      expect(json.url?.skipped).not.toBeTruthy();
      expect(json.url?.httpCode).toEqual(0);
    });
  });

  test('Verify RPM search', async ({ client }) => {
    let rpmSearch: ApiSearchRpmResponse[];
    await test.step('Search small red hat repo', async () => {
      rpmSearch = await new RpmsApi(client).searchRpm({
        apiContentUnitSearchRequest: {
          search: 'gcc-plugin-devel',
          urls: [SmallRedHatRepoURL],
        },
      });
      const names = rpmSearch.map((v) => v.packageName);
      expect(names).toContain('gcc-plugin-devel');
    });
  });

  test('Add popular repository', async ({ client }) => {
    let popularRepos: ApiPopularRepositoriesCollectionResponse;
    await test.step('List popular repositories', async () => {
      popularRepos = await new PopularRepositoriesApi(client).listPopularRepositories({
        search: 'EPEL 9',
      });
      expect(popularRepos.meta?.count).toBe(1);
      expect(popularRepos.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
    });

    await test.step('Delete existing repository if exists', async () => {
      if (popularRepos?.data?.length && popularRepos?.data?.[0].uuid != '') {
        const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
          uuid: popularRepos.data[0].uuid?.toString(),
        });
        expect(resp.raw.status).toBe(204);
      }
    });

    await test.step('Create custom repository for popular repository', async () => {
      const repo = await new RepositoriesApi(client).createRepository(<CreateRepositoryRequest>{
        apiRepositoryRequest: {
          name: popularRepos.data?.[0].suggestedName,
          url: popularRepos.data?.[0].url,
          gpgKey: popularRepos.data?.[0].gpgKey,
        },
      });
      expect(repo.name).toBe(popularRepos.data?.[0].suggestedName);
    });

    await test.step('Get repo uuid', async () => {
      popularRepos = await new PopularRepositoriesApi(client).listPopularRepositories({
        search: 'EPEL 9',
      });
      expect(popularRepos.meta?.count).toBe(1);
      expect(popularRepos.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
    });

    await test.step('Delete repository', async () => {
      const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
        uuid: popularRepos.data?.[0].uuid?.toString(),
      });
      expect(resp.raw.status).toBe(204);
    });
  });

  test('Verify that repo creation ignores duplicate distro versions', async ({
    client,
    cleanup,
  }) => {
    const repoName = randomName();
    const repoUrl = randomUrl();
    const distroVersions = ['8'];
    const duplicatedDistroVersions = ['8', '8'];

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, repoUrl));

    await test.step(
      'Delete existing repository if exists',
      async () => {
        const existing = await new RepositoriesApi(client).listRepositories(<
          ListRepositoriesRequest
        >{
          url: repoUrl,
        });

        if (existing?.data?.length) {
          const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
            uuid: existing.data[0].uuid?.toString(),
          });
          expect(resp.raw.status).toBe(204);
        }
      },
      { box: true },
    );

    let repo: ApiRepositoryResponse;
    await test.step(`Create a repo with duplicate distro versions and check that the versions aren't duplicated`, async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          distributionArch: 's390x',
          distributionVersions: duplicatedDistroVersions,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.distributionVersions).toStrictEqual(distroVersions);
    });
  });

  test('Bulk import repositories', async ({ client, cleanup }) => {
    const repoNamePrefix = 'bulk-import-';
    const repoDict1 = {
      name: `bulk-import-${randomName()}`,
      url: randomUrl(),
      origin: 'external',
      snapshot: true,
    };
    const repoDict2 = {
      name: `bulk-import-${randomName()}`,
      url: randomUrl(),
      origin: 'external',
      snapshot: true,
    };
    await cleanup.runAndAdd(() => cleanupRepositories(client, repoNamePrefix));

    await test.step('Bulk import repositories', async () => {
      const importedRepos = await new RepositoriesApi(client).bulkImportRepositories({
        apiRepositoryRequest: [repoDict1, repoDict2],
      });

      expect(importedRepos[0].name).toBe(repoDict1.name);
      expect(importedRepos[1].name).toBe(repoDict2.name);

      for (const importedRepo of importedRepos) {
        const getRepository = () =>
          new RepositoriesApi(client).getRepository({ uuid: importedRepo.uuid ?? '' });
        const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
        const resp = await poll(getRepository, waitWhilePending, 10);
        expect(resp.status).toBe('Valid');
      }

      const respList = await new RepositoriesApi(client).listRepositories({
        search: repoNamePrefix,
        origin: 'external',
      });
      expect(respList.data?.length).toBe(importedRepos.length);
    });
  });

  test('Bulk export repositories', async ({ client, cleanup }) => {
    const repoNamePrefix = 'bulk-export-';
    const repositories = [
      {
        name: `bulk-export-${randomName()}`,
        url: randomUrl(),
        origin: 'external',
        snapshot: true,
      },
      {
        name: `bulk-export-${randomName()}`,
        url: randomUrl(),
        origin: 'external',
        snapshot: true,
      },
    ];

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoNamePrefix));

    await test.step('create repositories and export it', async () => {
      const createdRepositories = await new RepositoriesApi(client).bulkCreateRepositories({
        apiRepositoryRequest: repositories,
      });
      expect(createdRepositories.length).toBe(repositories.length);
      const exportResponse = await new RepositoriesApi(client).bulkExportRepositories({
        apiRepositoryExportRequest: {
          repositoryUuids: createdRepositories
            .map((repo) => repo.uuid)
            .filter((uuid): uuid is string => uuid !== undefined),
        },
      });
      expect(exportResponse.length).toBe(repositories.length);

      // Update the test to compare only relevant fields between createdRepositories and exportResponse
      expect(exportResponse.map(({ name, url, snapshot }) => ({ name, url, snapshot }))).toEqual(
        expect.arrayContaining(
          createdRepositories.map(({ name, url, snapshot }) => ({ name, url, snapshot })),
        ),
      );
    });
  });

  test('Verify "any" filter in repository list filter', async ({ client, cleanup }) => {
    const repo1Name = `repo1-arch-any-${randomName()}`;
    const repo2Name = `repo2-ver-any-${randomName()}`;
    const repo3Name = `repo3-arch-ver-${randomName()}`;

    // Using different test URLs to ensure uniqueness
    const repo1Url = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';
    const repo2Url = 'https://content-services.github.io/fixtures/yum/comps-modules/v2/';
    const repo3Url = 'https://content-services.github.io/fixtures/yum/centirepos/repo01/';

    await cleanup.runAndAdd(() => cleanupRepositories(client, repo1Name, repo2Name, repo3Name));

    let repo1: ApiRepositoryResponse;
    let repo2: ApiRepositoryResponse;
    let repo3: ApiRepositoryResponse;

    await test.step('Create repo1 with arch="any" and versions=["any"]', async () => {
      repo1 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repo1Name,
          url: repo1Url,
          distributionArch: 'any',
          distributionVersions: ['any'],
        },
      });
      expect(repo1.name).toBe(repo1Name);
      expect(repo1.distributionArch).toBe('any');
      expect(repo1.distributionVersions).toEqual(['any']);
    });

    await test.step('Wait for repo1 introspection to complete', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo1.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
    });

    await test.step('Create repo2 with arch="any" and versions=["any"]', async () => {
      repo2 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repo2Name,
          url: repo2Url,
          distributionArch: 'any',
          distributionVersions: ['any'],
        },
      });
      expect(repo2.name).toBe(repo2Name);
      expect(repo2.distributionArch).toBe('any');
      expect(repo2.distributionVersions).toEqual(['any']);
    });

    await test.step('Wait for repo2 introspection to complete', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo2.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
    });

    await test.step('Create repo3 with arch="x86_64" and versions=["7"]', async () => {
      repo3 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repo3Name,
          url: repo3Url,
          distributionArch: 'x86_64',
          distributionVersions: ['7'],
        },
      });
      expect(repo3.name).toBe(repo3Name);
      expect(repo3.distributionArch).toBe('x86_64');
      expect(repo3.distributionVersions).toEqual(['7']);
    });

    await test.step('Wait for repo3 introspection to complete', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo3.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
    });

    await test.step('Apply "any" filter on distribution_arch', async () => {
      const anyFilterResultArch = await new RepositoriesApi(client).listRepositories(<
        ListRepositoriesRequest
      >{
        availableForArch: 'any',
      });

      // Check that our "any" arch repos are found in the results
      const repoUuids = anyFilterResultArch.data?.map((repo) => repo.uuid) || [];
      expect(repoUuids).toContain(repo1.uuid);
      expect(repoUuids).toContain(repo2.uuid);

      // Verify every repo in the result has "any" as dist_arch
      anyFilterResultArch.data?.forEach((repo) => {
        expect(repo.distributionArch).toBe('any');
      });
    });

    await test.step('Apply "any" filter on available_for_version', async () => {
      const anyFilterResultVersion = await new RepositoriesApi(client).listRepositories(<
        ListRepositoriesRequest
      >{
        availableForVersion: 'any',
      });

      // Check that our "any" version repos are found in the results
      const repoUuids = anyFilterResultVersion.data?.map((repo) => repo.uuid) || [];
      expect(repoUuids).toContain(repo1.uuid);
      expect(repoUuids).toContain(repo2.uuid);

      // Verify every repo in the result has "any" as dist_version
      anyFilterResultVersion.data?.forEach((repo) => {
        expect(repo.distributionVersions).toEqual(['any']);
      });
    });

    await test.step('Verify non-"any" repo is not found in "any" filter results', async () => {
      const anyFilterResultArch = await new RepositoriesApi(client).listRepositories(<
        ListRepositoriesRequest
      >{
        availableForArch: 'any',
      });

      const anyFilterResultVersion = await new RepositoriesApi(client).listRepositories(<
        ListRepositoriesRequest
      >{
        availableForVersion: 'any',
      });

      const archFilterUuids = anyFilterResultArch.data?.map((repo) => repo.uuid) || [];
      const versionFilterUuids = anyFilterResultVersion.data?.map((repo) => repo.uuid) || [];

      // repo3 should not be found in either "any" filter result
      expect(archFilterUuids).not.toContain(repo3.uuid);
      expect(versionFilterUuids).not.toContain(repo3.uuid);
    });
  });

  test('Fetch and verify repository configuration file', async ({ client, cleanup }) => {
    const repoName = `test-repo-config-${randomName()}`;
    const repoUrl = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, repoUrl));

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with snapshot enabled', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: true,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
      expect(repo.snapshot).toBe(true);
    });

    await test.step('Wait for repository to be valid', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
      expect(resp.lastSnapshotUuid).toBeDefined();
      // Update repo with the full response including snapshot info
      repo = resp;
    });

    await test.step('Get repo configuration file and verify contents', async () => {
      expect(repo.lastSnapshotUuid).toBeDefined();

      const configFile = await new RepositoriesApi(client).getRepoConfigurationFile({
        snapshotUuid: repo.lastSnapshotUuid!,
      });

      // Verify the config file contains expected elements
      expect(configFile).toContain(repo.name);
      expect(configFile).toContain('sslclientcert=/etc/pki/consumer/cert.pem');
      expect(configFile).toContain('sslclientkey=/etc/pki/consumer/key.pem');

      // Extract baseurl from config file
      const baseurlMatch = configFile.match(/baseurl=(.*)/);
      expect(baseurlMatch).not.toBeNull();

      if (baseurlMatch) {
        const baseurl = baseurlMatch[1];

        // Verify repo UUID is in the baseurl
        expect(baseurl).toContain(repo.uuid);
        // Verify the baseurl matches the last snapshot URL
        expect(repo.lastSnapshot?.url).toBe(baseurl);
      }
    });
  });

  test('Manually trigger repository snapshot', async ({ client, cleanup }) => {
    const repoName = `manual-snapshot-${randomName()}`;
    const repoUrl = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, repoUrl));

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with snapshot enabled', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: true,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
      expect(repo.snapshot).toBe(true);
    });

    let FirstRepo: ApiRepositoryResponse;
    await test.step('Wait for repository to be valid and get initial snapshot info', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
      repo = resp;

      FirstRepo = await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid?.toString(),
      });
      expect(FirstRepo.lastSnapshotTaskUuid).toBeDefined();
    });

    let currentTime: Date;
    await test.step('Note current time for snapshot timestamp verification', async () => {
      currentTime = new Date();
      currentTime.setMilliseconds(0);
    });

    await test.step('Trigger manual snapshot', async () => {
      const snapshotTask = await new RepositoriesApi(client).createSnapshot(<CreateSnapshotRequest>{
        uuid: repo.uuid!,
      });
      expect(snapshotTask.uuid).toBeDefined();
    });

    await test.step('Wait for manual snapshot task to complete', async () => {
      // Get the repository to get the latest task UUID
      const latestRepo = await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid?.toString(),
      });

      expect(latestRepo.lastSnapshotTaskUuid).toBeDefined();
      expect(latestRepo.lastSnapshotTaskUuid).not.toBe(FirstRepo.lastSnapshotTaskUuid);
      const taskUuid = latestRepo.lastSnapshotTaskUuid!;

      // Wait for the task to complete
      const getTask = () =>
        new TasksApi(client).getTask(<GetTaskRequest>{
          uuid: taskUuid,
        });
      const waitWhileRunning = (task: ApiTaskInfoResponse) => task.status !== 'completed';
      const completedTask = await poll(getTask, waitWhileRunning, 10);

      expect(completedTask.status).toBe('completed');

      // Verify the task was created at or after our noted time
      const taskCreatedAt = new Date(completedTask.createdAt!);
      expect(taskCreatedAt.getTime()).toBeGreaterThanOrEqual(currentTime.getTime());
    });
  });
});
