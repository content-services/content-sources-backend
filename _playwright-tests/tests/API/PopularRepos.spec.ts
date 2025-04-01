import { expect, test } from 'test-utils';
import { PopularRepositoriesApi } from 'test-utils/client';

test.describe('Popular repositories', () => {
  test('List popular repositories', async ({ client }) => {
    const resp = await new PopularRepositoriesApi(client).listPopularRepositories({
      search: 'EPEL 9',
    });
    expect(resp.meta?.count).toBe(1);
    expect(resp.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
  });
});
