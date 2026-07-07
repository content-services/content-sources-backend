import { mergeTests } from '@playwright/test';
import { blockAnalyticsTest } from './blockAnalytics';
import { browserConsoleTest } from './browserConsole';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { coverageTest } from './coverage';
import { currentsTest } from './currents';
import { databaseTest } from './db';
import { tokenRefreshTest } from './tokenRefresh';
import { unusedRepoUrlTest } from './unusedRepoUrl';

const sharedTest = mergeTests(
  currentsTest,
  blockAnalyticsTest,
  clientTest,
  cleanupTest,
  coverageTest,
  databaseTest,
  tokenRefreshTest,
  unusedRepoUrlTest,
);

export const test =
  process.env.CAPTURE_BROWSER_CONSOLE === 'true'
    ? mergeTests(sharedTest, browserConsoleTest)
    : sharedTest;

export { expect } from '@playwright/test';
