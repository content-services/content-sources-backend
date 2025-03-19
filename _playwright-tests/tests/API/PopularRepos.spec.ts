import { expect } from '@playwright/test';
import { test } from './base_client';
import { PopularRepositoriesApi } from './client';

test.describe('Popular repositories', () => {
  test('List popular repositories', async ({ client }) => {
    const resp = await new PopularRepositoriesApi(client).listPopularRepositories({
      search: 'EPEL 9',
    });
    expect(resp.meta?.count).toBe(1);
    expect(resp.data?.[0].suggestedName).toBe('EPEL 9 Everything x86_64');
  });
});
