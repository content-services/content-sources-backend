import { expect } from '@playwright/test';
import {test, expectError} from "./base_client";
import {FeaturesApi, PopularRepositoriesApi, ResponseError, TemplatesApi} from "./client";
import { ListPopularRepositoriesRequest } from './client/apis/PopularRepositoriesApi';


test('Content > Create Bad Template', async ({ client }) => {
    await expectError(400, "Name cannot be blank", 
        new TemplatesApi(client).createTemplate({apiTemplateRequest: {name: "", arch: "", repositoryUuids: [], version: ""}})
    );
});