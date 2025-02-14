import { expect } from '@playwright/test';

import { test } from "./base_client";
import { TemplatesApi, RepositoriesApi, GetRepositoryRequest, ApiRepositoryResponse } from "./client";
import { deleteAllRepos } from './helpers/deleteRepositories';
import { randomName } from './helpers/repoHelpers';
import {poll} from "./helpers/apiHelpers";


test('TemplateCRUD', async ({ client }) => {

	const repo_uuid = await test.step('Create test repo', async () => {
	  const repo_name = `Test-repo-from-api-${randomName()}`
	  console.log("repo_name:", repo_name)
	  const repo = await new RepositoriesApi(client).createRepository({apiRepositoryRequest: {name: `${repo_name}`, snapshot: true, url: "https://stephenw.fedorapeople.org/multirepos/9/repo09/"}});
	  expect(repo.name).toContain("Test-repo-from-api" )
      console.log(repo.uuid)
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
       const template_name = `Test-template-from-api-${randomName()}`
	   const template = await new TemplatesApi(client).createTemplate({apiTemplateRequest: {name: `${template_name}`, arch: "x86_64", repositoryUuids: [`${repo_uuid}`], version: "9", description: "Created the template"}})
	   expect(template.name).toContain('Test-template-from-api')
	   const template_uuid = template.uuid
	   console.log("template.uuid is `${template.uuid}`")
	   return template_uuid
    });
   
    await test.step('Read a Template', async () => {
		const resp = await new TemplatesApi(client).getTemplate({uuid: `${template_uuid}`})
        console.log(resp)
	});

    await test.step('Update a Template', async () => {
        const resp = new TemplatesApi(client).partialUpdateTemplate(`${template_uuid}`,{ description: "Updated the template"})
        expect ((await resp).description).toBe('Updated the template')
    });

    await test.step('Delete a Template', async () => {
		const resp = new TemplatesApi(client).deleteTemplate(`${template_uuid}`)
        console.log(resp);
	})
 });
