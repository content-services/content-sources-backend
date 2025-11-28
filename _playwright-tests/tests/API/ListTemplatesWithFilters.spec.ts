import { cleanupRepositories, cleanupTemplates, expect, randomName, test } from 'test-utils';
import { ApiRepositoryResponse, ApiTemplateResponse, RepositoriesApi, TemplatesApi } from "test-utils/client";
import { waitWhileRepositoryIsPending } from 'test-utils/helpers';


test.describe('List Templates With Filters', () => {
    test('List Templates With Filters', async ({ client, cleanup }) => {
        const baseRepoName = `Repo-for-template-filtering`;
        const templateNamePrefix = `Template-for-template-filtering`;
        const createdTemplates: ApiTemplateResponse[] = [];
        const createdRepos: ApiRepositoryResponse[] = [];
        await cleanup.runAndAdd(() => cleanupTemplates(client, templateNamePrefix));
        await cleanup.runAndAdd(() => cleanupRepositories(client, baseRepoName,
            'https://content-services.github.io/fixtures/yum/centirepos/repo02/',
            'https://content-services.github.io/fixtures/yum/centirepos/repo03/',
        ));

        await test.step('Create test repositories and templates for filtering', async () => {
            const repo1 = await new RepositoriesApi(client).createRepository({
                apiRepositoryRequest: {
                    name: `${baseRepoName}-${randomName()}-1`,
                    url: 'https://content-services.github.io/fixtures/yum/centirepos/repo02/',
                    distributionArch: 'any',
                    distributionVersions: ['any'],
                    snapshot: true,
                },
            });
            createdRepos.push(repo1);

            const repo2 = await new RepositoriesApi(client).createRepository({
                apiRepositoryRequest: {
                    name: `${baseRepoName}-${randomName()}-2`,
                    url: 'https://content-services.github.io/fixtures/yum/centirepos/repo03/',
                    distributionArch: 'any',
                    distributionVersions: ['any'],
                    snapshot: true,
                },
            });
            createdRepos.push(repo2);
            const resp1 = await waitWhileRepositoryIsPending(client, repo1.uuid!.toString());
            await expect(resp1.status).toBe('Valid');
            const resp2 = await waitWhileRepositoryIsPending(client, repo2.uuid!.toString());
            await expect(resp2.status).toBe('Valid');

            const template1 = await new TemplatesApi(client).createTemplate({
                apiTemplateRequest: {
                    name: `${templateNamePrefix}-${randomName()}-1`,
                    repositoryUuids: [`${repo1.uuid!}`],
                    arch: 'x86_64',
                    version: '8',
                },
            });
            const fetchedTemplate1 = await new TemplatesApi(client).getTemplate({ uuid: template1.uuid! });
            createdTemplates.push(fetchedTemplate1);

            const template2 = await new TemplatesApi(client).createTemplate({
                apiTemplateRequest: {
                    name: `${templateNamePrefix}-${randomName()}-2`,
                    repositoryUuids: [`${repo1.uuid!}`],
                    arch: 'aarch64',
                    version: '9',
                },
            });
            const fetchedTemplate2 = await new TemplatesApi(client).getTemplate({ uuid: template2.uuid! });
            createdTemplates.push(fetchedTemplate2);

            const template3 = await new TemplatesApi(client).createTemplate({
                apiTemplateRequest: {
                    name: `${templateNamePrefix}-${randomName()}-3`,
                    repositoryUuids: [`${repo2.uuid!}`],
                    arch: 'x86_64',
                    version: '10',
                },
            });
            const fetchedTemplate3 = await new TemplatesApi(client).getTemplate({ uuid: template3.uuid! });
            createdTemplates.push(fetchedTemplate3);

            const template4 = await new TemplatesApi(client).createTemplate({
                apiTemplateRequest: {
                    name: `${templateNamePrefix}-${randomName()}-4`,
                    repositoryUuids: [`${repo2.uuid!}`],
                    arch: 'aarch64',
                    version: '9',
                },
            });
            const fetchedTemplate4 = await new TemplatesApi(client).getTemplate({ uuid: template4.uuid! });
            createdTemplates.push(fetchedTemplate4);
        });

        const sortedTemplates = createdTemplates.sort((a, b) => a.name!.localeCompare(b.name!));
        const sortedRepos = createdRepos.sort((a, b) => a.name!.localeCompare(b.name!));

        await test.step('List templates by name', async () => {
            const templates_name_search = await new TemplatesApi(client).listTemplates({
                name: sortedTemplates[0].name,
            });
            expect(templates_name_search.data).toHaveLength(1);
            expect(templates_name_search.data?.[0]?.name).toBe(sortedTemplates[0].name);
        });

        await test.step('List templates by architecture', async () => {
            const templates_arch_search = await new TemplatesApi(client).listTemplates({
                arch: 'x86_64',
            });
            expect(templates_arch_search.data).toHaveLength(2);
            expect(templates_arch_search.data?.map(t => t.name)).toEqual(sortedTemplates.filter(t => t.arch === 'x86_64').map(t => t.name));
        });

        await test.step('List templates by version', async () => {
            const templates_version_search = await new TemplatesApi(client).listTemplates({
                version: '10',
            });
            expect(templates_version_search.data).toHaveLength(1);
            expect(templates_version_search.data?.[0]?.name).toBe(sortedTemplates.find(t => t.version === '10')?.name);
            expect(templates_version_search.data?.[0]?.version).toBe('10');
        });

        await test.step('List templates by architecture, version, limit, offset, sortby', async () => {
            const templates_mixed_search = await new TemplatesApi(client).listTemplates({
                arch: 'aarch64',
                version: '9',
                limit: 1,
                offset: 0,
                sortBy: 'name',
            });

            const expectedTemplate = sortedTemplates
                .find(t => t.arch === 'aarch64' && t.version === '9')!;

            expect(templates_mixed_search.data).toHaveLength(1);
            expect(templates_mixed_search.data?.[0]?.name).toBe(expectedTemplate.name);
        });

        await test.step('List templates by repository UUIDs', async () => {
            const templates_repository_uuids_search = await new TemplatesApi(client).listTemplates({
                repositoryUuids: `${sortedRepos[0].uuid},${sortedRepos[1].uuid}`,
                sortBy: 'name',
            });
            expect(templates_repository_uuids_search.data).toHaveLength(4);
            expect(templates_repository_uuids_search.data?.map(t => t.name)).toEqual(sortedTemplates.map(t => t.name));
        });

        await test.step('List templates by snapshot UUIDs', async () => {
            await expect(sortedTemplates[0].snapshots?.[0]?.uuid).toBeDefined();
            const targetRepoUuid = sortedTemplates[0].repositoryUuids?.[0];
            const expectedTemplates = sortedTemplates.filter(t => t.repositoryUuids?.[0] === targetRepoUuid);

            const templates_snapshot_uuids_search = await new TemplatesApi(client).listTemplates({
                snapshotUuids: `${sortedTemplates[0].snapshots![0]!.uuid!}`,
            });
            expect(templates_snapshot_uuids_search.data).toHaveLength(2);
            expect(templates_snapshot_uuids_search.data?.map(t => t.name)).toEqual(expectedTemplates.map(t => t.name));
        });

        await test.step('List templates by limit', async () => {
            const templates_limit_search = await new TemplatesApi(client).listTemplates({
                limit: 2,
            });
            
            expect(templates_limit_search.data).toHaveLength(2);

        });

        await test.step('List templates by offset', async () => {
            const templates_offset_search = await new TemplatesApi(client).listTemplates({
                offset: 1,
                sortBy: 'name',
            });
            expect(templates_offset_search.data).toHaveLength(3);

            const sorted_names = sortedTemplates
                .map(t => t.name)
                .slice(1);

            expect(templates_offset_search.data?.map(t => t.name)).toEqual(sorted_names);
        });
    });
});