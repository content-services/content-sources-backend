import { mergeTests } from '@playwright/test';
import { blockAnalyticsTest } from './blockAnalytics';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { coverageTest } from './coverage';
import { databaseTest } from './db';
import { unusedRepoUrlTest } from './unusedRepoUrl';
import { tokenRefreshTest } from './tokenRefresh';

export const test = mergeTests(
  blockAnalyticsTest,
  clientTest,
  cleanupTest,
  coverageTest,
  databaseTest,
  unusedRepoUrlTest,
  tokenRefreshTest,
);

export { expect } from '@playwright/test';
