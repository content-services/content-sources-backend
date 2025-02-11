import { test, expectError } from "./base_client";
import { TemplatesApi } from "./client";


test('Content > Create Bad Template', async ({ client }) => {
    await expectError(400, "Name cannot be blank", 
        new TemplatesApi(client).createTemplate({apiTemplateRequest: {name: "", arch: "", repositoryUuids: [], version: ""}})
    );
});