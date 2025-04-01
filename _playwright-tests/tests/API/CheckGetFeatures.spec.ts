import { expect, test } from 'test-utils';
import { FeaturesApi } from 'test-utils/client';

test.describe('Features', () => {
  test('List features', async ({ client }) => {
    const resp = await new FeaturesApi(client).listFeatures();
    expect(resp['snapshots']['enabled']).toBe(true);
  });
});
