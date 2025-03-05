import { test } from './base_client';
import {
  RepositoriesApi,
  GetRepositoryRequest,
  ApiRepositoryResponse,
  ApiRepositoryValidationResponseFromJSON,
  type ListRepositoriesRequest,
  type ValidateRepositoryParametersRequest,
  RpmsApi,
  ApiSearchRpmResponse,
} from './client';
import { randomUUID } from 'crypto';
import { expect } from '@playwright/test';
import { poll, SmallRedHatRepoURL } from './helpers/apiHelpers';

test.describe('Repositories', () => {
  test('Verify repository introspection', async ({ client }) => {
    await test.step('delete existing repository if exists', async () => {
      const existing = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
        search: 'test-repository',
      });

      if (existing?.data?.length) {
        const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
          uuid: existing.data[0].uuid?.toString(),
        });
        expect(resp.raw.status).toBe(204);
      }
    });

    let repo: ApiRepositoryResponse;
    await test.step('Create a repo with name test-repository', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: 'test-repository',
          url: 'https://content-services.github.io/fixtures/yum/comps-modules/v1/',
        },
      });
      expect(repo.name).toBe('test-repository');
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

  test('Validate repository parameters', async ({ client }) => {
    const invalidFormatUuid = '49742069-edff-f58f-a2dd-5eb068444888';
    const repoName = randomUUID();
    const repoUrl = 'https://content-services.github.io/fixtures/yum/comps-modules/v2/';
    const realButBadRepoUrl = 'http://jlsherrill.fedorapeople.org/fake-repos/';

    await test.step('Delete existing repository if exists', async () => {
      const existing = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
        url: repoUrl,
      });

      if (existing?.data?.length) {
        const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
          uuid: existing.data[0].uuid?.toString(),
        });
        expect(resp.raw.status).toBe(204);
      }
    });

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
});
