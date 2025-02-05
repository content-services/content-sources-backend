import { test, expect } from '@playwright/test';

test('Content > GetFeatures API', async ({ request }) => {
    const result = await request.get('/api/content-sources/v1/features/');
    expect(result.status()).toBe(200);
});
