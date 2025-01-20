import { test, expect } from '@playwright/test';

test('Content > getFeatures API', async ({ request }) => {
    const result = await request.get('/api/content-sources/v1/repositories/');
    expect(result.status()).toBe(200);
});
