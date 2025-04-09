import { expectError, test } from 'test-utils';
import { TemplatesApi } from 'test-utils/client';

test.describe('Templates', () => {
  test('Create bad template', async ({ client }) => {
    await expectError(
      400,
      'Name cannot be blank',
      new TemplatesApi(client).createTemplate({
        apiTemplateRequest: { name: '', arch: '', repositoryUuids: [], version: '' },
      }),
    );
  });
});
