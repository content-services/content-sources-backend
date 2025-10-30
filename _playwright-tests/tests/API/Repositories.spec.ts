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
  FeaturesApi,
  Configuration,
} from 'test-utils/client';
import {
  cleanupRepositories,
  poll,
  randomName,
  randomUrl,
  SmallRedHatRepoURL,
  TestGpgKey,
  waitWhileRepositoryIsPending,
} from 'test-utils/helpers';
import { setAuthorizationHeader } from '../helpers/loginHelpers';

test.describe('Repositories', () => {
  test('Verify repository introspection', async ({ client, cleanup }) => {
    const testStartTime = new Date();
    const repoName = `verify-repository-introspection-${randomName()}`;
    const repoUrl = randomUrl();

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
      const resp = await waitWhileRepositoryIsPending(client, repo.uuid!.toString());
      expect(resp.status).toBe('Valid');

      expect(resp.lastIntrospectionTime).toBeDefined();
      expect(resp.lastIntrospectionError).toBe('');

      const introspectionTime = new Date(resp.lastIntrospectionTime!);
      expect(introspectionTime.getTime()).not.toBeNaN(); // Valid date format
      expect(introspectionTime.getTime()).toBeGreaterThan(testStartTime.getTime() - 1500); // 1.5 second buffer for timing precision
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

  test('Validate GPG key with repository parameters', async ({ client }) => {
    const repoUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';
    const invalidGpgKey = 'not-a-valid-gpg-key';

    await test.step('Validate repository URL with valid GPG key', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            url: repoUrl,
            gpgKey: TestGpgKey,
          },
        ],
      });

      expect(resp[0].url?.httpCode).toBe(200);
      expect(resp[0].url?.metadataPresent).toBeTruthy();
      expect(resp[0].gpgKey).toBeDefined();
      expect(resp[0].gpgKey?.valid).toBeTruthy();
    });

    await test.step('Validate repository URL with invalid GPG key format', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            url: repoUrl,
            gpgKey: invalidGpgKey,
          },
        ],
      });

      expect(resp[0].url?.httpCode).toBe(200);
      expect(resp[0].url?.metadataPresent).toBeTruthy();
      expect(resp[0].gpgKey).toBeDefined();
      expect(resp[0].gpgKey?.valid).not.toBeTruthy();
      expect(resp[0].gpgKey?.error).toBeDefined();
    });

    await test.step('Validate repository URL with empty GPG key', async () => {
      const resp = await new RepositoriesApi(client).validateRepositoryParameters(<
        ValidateRepositoryParametersRequest
      >{
        apiRepositoryValidationRequest: [
          {
            url: repoUrl,
            gpgKey: '',
          },
        ],
      });

      expect(resp[0].url?.httpCode).toBe(200);
      expect(resp[0].url?.metadataPresent).toBeTruthy();
      expect(resp[0].gpgKey).toBeDefined();
      expect(resp[0].gpgKey?.valid).toBeTruthy();
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
    const features = await new FeaturesApi(client).listFeatures();
    test.skip(
      !!features['communityrepos']?.enabled && !features['allowcustomepelcreation']?.enabled,
      'Allow custom EPEL creation feature is not enabled, skipping test',
    );

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
      const resp = await waitWhileRepositoryIsPending(client, repo.uuid!.toString());
      expect(resp.status).toBe('Valid');
      expect(resp.failedIntrospectionsCount).toBe(0);
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

  test('Bulk create repositories', async ({ client, cleanup }) => {
    const repoNamePrefix = 'bulk-create-';
    const name1 = `${repoNamePrefix}${randomName()}`;
    const url1 = randomUrl();
    const name2 = `${repoNamePrefix}${randomName()}`;
    const url2 = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoNamePrefix));

    let createdRepos: ApiRepositoryResponse[];
    await test.step('Use bulk create endpoint to create two repos', async () => {
      createdRepos = await new RepositoriesApi(client).bulkCreateRepositories({
        apiRepositoryRequest: [
          {
            name: name1,
            url: url1,
          },
          {
            name: name2,
            url: url2,
          },
        ],
      });

      expect(createdRepos.length).toBe(2);
      expect(createdRepos[0].name).toBe(name1);
      expect(createdRepos[0].url).toBe(url1);
      expect(createdRepos[1].name).toBe(name2);
      expect(createdRepos[1].url).toBe(url2);
    });

    await test.step('Verify both repos exist by listing repositories', async () => {
      const responseList = await new RepositoriesApi(client).listRepositories({
        origin: 'external',
      });

      const repoNames = responseList.data?.map((repo) => repo.name) || [];
      expect(repoNames).toContain(name1);
      expect(repoNames).toContain(name2);
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
        const resp = await waitWhileRepositoryIsPending(client, importedRepo.uuid!.toString());
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
      const resp = await waitWhileRepositoryIsPending(client, repo1.uuid!.toString());
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
      const resp = await waitWhileRepositoryIsPending(client, repo2.uuid!.toString());
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
      const resp = await waitWhileRepositoryIsPending(client, repo3.uuid!.toString());
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
      const resp = await waitWhileRepositoryIsPending(client, repo.uuid!.toString());
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

    let firstSnapshotTaskUuid: string | undefined;
    await test.step('Wait for repository to be valid and get initial snapshot info', async () => {
      const resp = await waitWhileRepositoryIsPending(client, repo.uuid!.toString());
      expect(resp.status).toBe('Valid');
      repo = resp;
      firstSnapshotTaskUuid = repo.lastSnapshotTaskUuid;
      expect(repo.lastSnapshotTaskUuid).toBeDefined();
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
      expect(latestRepo.lastSnapshotTaskUuid).not.toBe(firstSnapshotTaskUuid);
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

  test('Partial update repository', async ({ client, cleanup }) => {
    const repoName = `patch-repository-${randomName()}`;
    const repoUrl = randomUrl();
    const updatedName = `updated-${repoName}`;
    const updatedUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v1/';

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, updatedName));

    let repo: ApiRepositoryResponse;
    await test.step('Create a repository for updating', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          distributionArch: 'x86_64',
          distributionVersions: ['8'],
          metadataVerification: true,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
      expect(repo.distributionArch).toBe('x86_64');
      expect(repo.distributionVersions).toEqual(['8']);
      expect(repo.metadataVerification).toBe(true);
    });

    await test.step('Update repository name and URL', async () => {
      const updatedRepo = await new RepositoriesApi(client).partialUpdateRepository({
        uuid: repo.uuid!,
        apiRepositoryUpdateRequest: {
          name: updatedName,
          url: updatedUrl,
        },
      });

      expect(updatedRepo.uuid).toBe(repo.uuid);
      expect(updatedRepo.name).toBe(updatedName);
      expect(updatedRepo.url).toBe(updatedUrl);
      expect(updatedRepo.distributionArch).toBe('x86_64');
      expect(updatedRepo.distributionVersions).toEqual(['8']);
      expect(updatedRepo.moduleHotfixes).toBe(false);
      expect(updatedRepo.gpgKey).toBe('');
    });

    await test.step('Update distributions, modules, metadata, and gpgKey fields', async () => {
      const updatedRepo = await new RepositoriesApi(client).partialUpdateRepository({
        uuid: repo.uuid!,
        apiRepositoryUpdateRequest: {
          distributionArch: 'any',
          distributionVersions: ['any'],
          metadataVerification: false,
          moduleHotfixes: true,
          gpgKey: 'test-gpg-key',
        },
      });

      expect(updatedRepo.uuid).toBe(repo.uuid);
      expect(updatedRepo.name).toBe(updatedName);
      expect(updatedRepo.url).toBe(updatedUrl);
      expect(updatedRepo.distributionArch).toBe('any');
      expect(updatedRepo.distributionVersions).toEqual(['any']);
      expect(updatedRepo.metadataVerification).toBe(false);
      expect(updatedRepo.moduleHotfixes).toBe(true);
      expect(updatedRepo.gpgKey).toBe('test-gpg-key');
    });
  });

  test('CRUD honours OrgId', async ({ cleanup }) => {
    const DefaultOrg = 99999;
    const DefaultUser = 'BananaMan';
    const baseUrl = process.env.BASE_URL;

    await setAuthorizationHeader(DefaultUser, DefaultOrg);
    const defaultUserHeader = process.env.IDENTITY_HEADER!;
    const defaultUserClient = new Configuration({
      basePath: baseUrl + '/api/content-sources/v1',
      headers: { 'x-rh-identity': defaultUserHeader },
    });

    const expectedOrgId = '99999';
    const repoName = `crud-orgid-test-${randomName()}`;
    const repoUrl = randomUrl();
    const updatedArch = 's390x';

    await cleanup.runAndAdd(() => cleanupRepositories(defaultUserClient, repoName, repoUrl));

    let repo: ApiRepositoryResponse;
    await test.step('Create repo', async () => {
      repo = await new RepositoriesApi(defaultUserClient).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          distributionArch: 'x86_64',
          distributionVersions: ['8'],
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
      expect(repo.orgId).toBe(expectedOrgId);
    });

    await test.step('Read repo', async () => {
      const fetchedRepo = await new RepositoriesApi(defaultUserClient).getRepository({
        uuid: repo.uuid!,
      });

      expect(fetchedRepo.orgId).toBe(expectedOrgId);
      expect(fetchedRepo.orgId).toBe(repo.orgId);
    });

    await test.step('Update the repo with a new arch (using PUT)', async () => {
      const updatedRepo = await new RepositoriesApi(defaultUserClient).fullUpdateRepository({
        uuid: repo.uuid!,
        apiRepositoryRequest: {
          name: repo.name!,
          url: repo.url!,
          distributionArch: updatedArch,
          distributionVersions: repo.distributionVersions!,
          metadataVerification: repo.metadataVerification,
          moduleHotfixes: repo.moduleHotfixes,
          snapshot: repo.snapshot,
          gpgKey: repo.gpgKey,
        },
      });

      expect(updatedRepo.distributionArch).toBe(updatedArch);
      expect(updatedRepo.orgId).toBe(expectedOrgId);
      expect(updatedRepo.orgId).toBe(repo.orgId);
      expect(updatedRepo.uuid).toBe(repo.uuid);

      repo = updatedRepo;
    });
  });

  test('Two users in different orgs can create the same repo', async ({ cleanup }) => {
    /**
     * This test verifies that repository uniqueness constraints are scoped per organization.
     * Two users in different orgs should be able to create repos with identical names and URLs.
     */
    const repoName = 'test-repo-unique-org';
    const repoUrl = 'https://yum.theforeman.org/pulpcore/3.4/el7/x86_64/';
    const DefaultOrg = 99999;
    const DefaultUser = 'BananaMan';
    const AlternateOrg = 88888;
    const AlternateUser = 'KiwiMan';
    const baseUrl = process.env.BASE_URL;

    // Get auth headers for both users
    await setAuthorizationHeader(DefaultUser, DefaultOrg);
    const defaultUserHeader = process.env.IDENTITY_HEADER!;

    await setAuthorizationHeader(AlternateUser, AlternateOrg);
    const alternateUserHeader = process.env.IDENTITY_HEADER!;

    // Create separate client configurations for each user
    const defaultUserClient = new Configuration({
      basePath: baseUrl + '/api/content-sources/v1',
      headers: { 'x-rh-identity': defaultUserHeader },
    });

    const alternateUserClient = new Configuration({
      basePath: baseUrl + '/api/content-sources/v1',
      headers: { 'x-rh-identity': alternateUserHeader },
    });

    // Add cleanup for both users' repositories
    await cleanup.runAndAdd(() => cleanupRepositories(defaultUserClient, repoName, repoUrl));
    await cleanup.runAndAdd(() => cleanupRepositories(alternateUserClient, repoName, repoUrl));

    let repo1: ApiRepositoryResponse;
    await test.step('Create repo with default user (BananaMan)', async () => {
      repo1 = await new RepositoriesApi(defaultUserClient).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: false,
        },
      });
      expect(repo1.name).toBe(repoName);
      expect(repo1.url).toBe(repoUrl);
    });

    await test.step('Wait for default user repository introspection to complete', async () => {
      const resp = await waitWhileRepositoryIsPending(defaultUserClient, repo1.uuid!.toString());
      expect(resp.status).toBe('Valid');
    });

    let repo2: ApiRepositoryResponse;
    await test.step('Create same repo with alternate user (KiwiMan) in different org', async () => {
      repo2 = await new RepositoriesApi(alternateUserClient).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: false,
        },
      });
      expect(repo2.name).toBe(repoName);
      expect(repo2.url).toBe(repoUrl);
      // Verify it's a different repository (different UUID)
      expect(repo2.uuid).not.toBe(repo1.uuid);
    });

    await test.step('Wait for alternate user repository introspection to complete', async () => {
      await waitWhileRepositoryIsPending(alternateUserClient, repo2.uuid!.toString());
    });
  });

  test('Full update repository', async ({ client, cleanup }) => {
    const repoName = `full-update-repo-${randomName()}`;
    const repoUrl = randomUrl();
    const updatedName = `updated-${repoName}`;
    const updatedUrl = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName, updatedName));

    let repo: ApiRepositoryResponse;
    await test.step('Create a repository', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          distributionArch: 'x86_64',
          distributionVersions: ['8'],
          gpgKey: 'test-gpg-key',
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
      expect(repo.distributionArch).toBe('x86_64');
      expect(repo.distributionVersions).toEqual(['8']);
      expect(repo.gpgKey).toBeDefined();

      await test.step('Update full repository', async () => {
        const updatedRepo = await new RepositoriesApi(client).fullUpdateRepository({
          uuid: repo.uuid!,
          apiRepositoryRequest: {
            name: updatedName,
            url: updatedUrl,
            distributionArch: 's390x',
            distributionVersions: ['9'],
          },
        });
        expect(updatedRepo.name).toBe(updatedName);
        expect(updatedRepo.url).toBe(updatedUrl);
        expect(updatedRepo.distributionArch).toBe('s390x');
        expect(updatedRepo.distributionVersions).toEqual(['9']);
        // when updating a repository, if you don't provide a gpgKey, it should be blank.
        expect(updatedRepo.gpgKey).toBe('');
      });
    });
  });

  test('Test listing all custom repos does not return rhel repos', async ({ client, cleanup }) => {
    const externalrepoNamePrefix = 'external-repo-';
    const externalName1 = `${externalrepoNamePrefix}${randomName()}`;
    const externalUrl1 = randomUrl();
    const externalName2 = `${externalrepoNamePrefix}${randomName()}`;
    const externalUrl2 = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, externalrepoNamePrefix));

    let createdRepos: ApiRepositoryResponse[];
    await test.step('Use bulk create endpoint to create two repos', async () => {
      createdRepos = await new RepositoriesApi(client).bulkCreateRepositories({
        apiRepositoryRequest: [
          {
            name: externalName1,
            url: externalUrl1,
          },
          {
            name: externalName2,
            url: externalUrl2,
          },
        ],
      });

      expect(createdRepos.length).toBe(2);
      const custom_repos = await new RepositoriesApi(client).listRepositories({
        origin: 'external',
      });
      expect(custom_repos.data!.length).toBeGreaterThan(0);
      expect(custom_repos.data!.every((repo) => repo.origin === 'external')).toBe(true);
      expect(custom_repos.data!.every((repo) => repo.origin !== 'red_hat')).toBe(true);

      const redhat_repos = await new RepositoriesApi(client).listRepositories({
        origin: 'red_hat',
      });
      expect(redhat_repos.data!.length).toBeGreaterThan(0);
      expect(redhat_repos.data!.every((repo) => repo.origin === 'red_hat')).toBe(true);
      expect(redhat_repos.data!.every((repo) => repo.origin !== 'external')).toBe(true);
    });
  });
});
