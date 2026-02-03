import {
  CurrentsFixtures,
  CurrentsWorkerFixtures,
  fixtures as currentsFixtures,
} from '@currents/playwright';
import { test as base } from '@playwright/test';

/**
 * Currents.dev fixtures for automatic coverage collection and upload.
 *
 * When tests use this fixture and Currents is configured in playwright.config.ts,
 * coverage data from window.__coverage__ will be automatically collected
 * and uploaded to Currents.dev.
 */
export const currentsTest = base.extend<CurrentsFixtures, CurrentsWorkerFixtures>({
  ...currentsFixtures.baseFixtures,
  ...currentsFixtures.coverageFixtures,
});
