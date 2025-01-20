import { defineConfig, devices } from '@playwright/test';
import path from 'path';

import { config } from 'dotenv'

config({ path: path.join(__dirname, './.env') })


/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
    testDir: './tests',
    /* Run tests in files in parallel */
    fullyParallel: false,
    /* Fail the build on CI if you accidentally left test.only in the source code. */
    forbidOnly: false,
    /* Retry on CI only */
    retries: 1,
    /* Opt out of parallel tests on CI. */
    workers: 1,
    /* Reporter to use. See https://playwright.dev/docs/test-reporters */
    reporter: !!process.env.CI ? [
        ["list"],
        [
            "playwright-ctrf-json-reporter",
            { useDetails: true, outputDir: "playwright-ctrf", outputFile: "playwright-ctrf.json" },
        ],
        ['html', { outputFolder: 'playwright-report' }]
    ] : "list",
    // This may need to be increased for different environments and applications.
    expect: { timeout: 15000 },
    /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
    use: {
        launchOptions: {
            args: ['--use-fake-device-for-media-stream'],
        },
        ...process.env.IDENTITY_HEADER ? {
            extraHTTPHeaders: {
                "x-rh-identity": process.env.IDENTITY_HEADER
            }
        } : {},
        /* Base URL to use in actions like `await page.goto('/')`. */
        // This is used for both the API and the UI navigation
        // This can be overridden in tests for external api's should that be needed.
        baseURL: process.env.BASE_URL,
        //We need to make sure the TOKEN exists before setting the extraHTTPHeader for the API
        /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
        trace: 'on-first-retry',
        ignoreHTTPSErrors: true,
    },
    /* Configure projects for major browsers */
    projects: [
        { name: 'setup', testMatch: /.*\.setup\.ts/, expect: { timeout: 20000 } },
        {
            name: 'Google Chrome',
            // testIgnore: /.*UploadRepo.spec.ts/,
            use: {
                ...devices['Desktop Chrome'],
            },
            dependencies: ['setup'],
        },
        // {
        //     // We need to use firefox for upload tests
        //     name: 'Firefox',
        //     // testMatch: [/.*UploadRepo.spec.ts/],
        //     use: {
        //         ...devices['Desktop Firefox'],  // Use prepared auth state.
        //         storageState: path.join(__dirname, './.auth/user.json'),
        //     },
        //     dependencies: ['setup'],
        // },
    ],

    /* Run your local dev server before starting the tests */
    // webServer: {
    //   command: 'npm run start',
    //   url: 'http://127.0.0.1:3000',
    //   reuseExistingServer: !process.env.CI,
    // },
});

