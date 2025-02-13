import { defineConfig, devices } from '@playwright/test';
import path from 'path';

import { config } from 'dotenv';

config({ path: path.join(__dirname, './.env') });

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: false,
  retries: 1,
  workers: 1,
  reporter: !!process.env.CI
    ? [
        ['list'],
        [
          'playwright-ctrf-json-reporter',
          { useDetails: true, outputDir: 'playwright-ctrf', outputFile: 'playwright-ctrf.json' },
        ],
        ['html', { outputFolder: 'playwright-report' }],
      ]
    : 'list',
  expect: { timeout: 15000 },
  use: {
    launchOptions: {
      args: ['--use-fake-device-for-media-stream'],
    },
    ...(process.env.IDENTITY_HEADER
      ? {
          extraHTTPHeaders: {
            'x-rh-identity': process.env.IDENTITY_HEADER,
          },
        }
      : {}),
    baseURL: process.env.BASE_URL,
    trace: 'on-first-retry',
    ignoreHTTPSErrors: true,
  },
  projects: [
    { name: 'setup', testMatch: /.*\.setup\.ts/, expect: { timeout: 20000 } },
    {
      name: 'Google Chrome',
      use: {
        ...devices['Desktop Chrome'],
      },
      dependencies: ['setup'],
    },
  ],
});
