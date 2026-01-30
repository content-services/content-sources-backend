import { test as base } from '@playwright/test';
import * as fs from 'fs/promises';
import * as path from 'path';

// Extend Window interface to include Istanbul's coverage object
declare global {
  interface Window {
    __coverage__?: Record<string, unknown>;
  }
}

const NYC_OUTPUT_DIR = path.join(process.cwd(), '.nyc_output');

/**
 * Coverage collection fixture for Playwright tests.
 * Automatically collects Istanbul coverage data from window.__coverage__
 * after each test when COVERAGE=true environment variable is set.
 */
export const coverageTest = base.extend({
  page: async ({ page }, use, testInfo) => {
    // Run the test
    await use(page);

    // After test: collect coverage if enabled
    if (process.env.COVERAGE === 'true') {
      try {
        const coverage = await page.evaluate(() => window.__coverage__ || null);

        if (coverage) {
          // Ensure output directory exists
          try {
            await fs.access(NYC_OUTPUT_DIR);
          } catch {
            await fs.mkdir(NYC_OUTPUT_DIR, { recursive: true });
          }

          // Sanitize test name for filename
          const sanitizedName = testInfo.title.replace(/[^a-zA-Z0-9-_]/g, '_');
          const timestamp = Date.now();
          const coverageFile = path.join(
            NYC_OUTPUT_DIR,
            `coverage-${sanitizedName}-${timestamp}.json`
          );

          await fs.writeFile(coverageFile, JSON.stringify(coverage));
        }
      } catch (error) {
        // Silently ignore coverage collection errors to not fail tests
        console.warn('Coverage collection failed:', error);
      }
    }
  },
});
