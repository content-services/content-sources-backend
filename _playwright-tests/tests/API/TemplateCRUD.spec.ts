import { expect } from '@playwright/test';

import { test } from "./base_client";
import { TemplatesApi, RepositoriesApi, GetRepositoryRequest, ApiRepositoryResponse, PartialUpdateTemplateRequest, DeleteTemplateRequest, ListTemplatesRequest, ListRepositoriesRequest, GetTemplateRequest } from "./client";
import { randomName, repo_url } from './helpers/repoHelpers';
import {poll} from "./helpers/apiHelpers";


test('TemplateCRUD', async ({ client }) => {

    await test.step('delete existing repository if it exists', async () => {
    const existing = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
      search: 'Test-repo-for-template-CRUD',
	 });

	if (existing?.data?.length) {
      const resp = await new RepositoriesApi(client).deleteRepositoryRaw(<GetRepositoryRequest>{
        uuid: existing.data[0].uuid?.toString(),
      });
      expect(resp.raw.status).toBe(204);
    }
    });

	await test.step('delete existing template if it exists', async () => {
    const existing = await new TemplatesApi(client).listTemplates(<ListTemplatesRequest>{
      search: 'Test-template-CRUD',
    });

	if (existing?.data?.length) {
      const resp = await new TemplatesApi(client).deleteTemplateRaw(<GetTemplateRequest>{
        uuid: existing.data[0].uuid?.toString(),
      });
      expect(resp.raw.status).toBe(204);
    }
    });

    const repo_uuid = await test.step('Create test repo', async () => {
	  const repo_name = `Test-repo-for-template-CRUD-${randomName()}`
	  const repo = await new RepositoriesApi(client).createRepository({apiRepositoryRequest: {name: `${repo_name}`, snapshot: true, url: `${repo_url}`}});
	  expect(repo.name).toContain("Test-repo-for-template-CRUD" )
	  const repo_uuid = repo.uuid
	  return repo_uuid
    });

	await test.step("wait for introspection to be completed", async () => {
        let getRepository = () => new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{uuid: repo_uuid?.toString()})
        let waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === "Pending"
        let resp = await poll(getRepository, waitWhilePending, 10)
        expect(resp.status).toBe("Valid")
    })

    const template_uuid = await test.step('Create a Template', async () => {
       const template_name = `Test-template-CRUD-${randomName()}`
	   const template = await new TemplatesApi(client).createTemplate({apiTemplateRequest: {name: `${template_name}`, arch: "x86_64", repositoryUuids: [`${repo_uuid}`], version: "9", description: "Created the template"}})
	   expect(template.name).toContain('Test-template-CRUD')
       const template_uuid = template.uuid
	   return template_uuid
    });
   
    await test.step('Read a Template', async () => {
		const resp = await new TemplatesApi(client).getTemplate({uuid: `${template_uuid}`})
	});


	await test.step('Update the template', async () => {
		const resp = await new TemplatesApi(client).partialUpdateTemplate(<PartialUpdateTemplateRequest>{uuid: template_uuid, apiTemplateUpdateRequest: {description: "Updated the template"}})
		expect (resp.description).toBe('Updated the template')
	});


    await test.step('Delete a Template', async () => {
		const resp = new TemplatesApi(client).deleteTemplate(<DeleteTemplateRequest>{uuid: `${template_uuid}`})
	})
 });
