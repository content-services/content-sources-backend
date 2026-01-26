import { test as base } from '@playwright/test';

/**
 * Fixture that blocks analytics requests to reduce noise in test traces.
 * This blocks Amplitude and other analytics services at the network level.
 */
export const blockAnalyticsTest = base.extend({
  page: async ({ page }, use) => {
    // Block Amplitude analytics requests
    await page.route(/amplitude\.com/, (route) => route.abort());

    // Block other common analytics services
    await page.route(/segment\.io/, (route) => route.abort());
    await page.route(/google-analytics\.com/, (route) => route.abort());
    await page.route(/googletagmanager\.com/, (route) => route.abort());
    await page.route('https://consent.trustarc.com/**', (route) => route.abort());
    await page.route('https://smetrics.redhat.com/**', (route) => route.abort());

    await use(page);
  },
});
