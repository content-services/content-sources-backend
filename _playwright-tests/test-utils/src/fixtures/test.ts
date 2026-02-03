import { mergeTests } from '@playwright/test';
import { blockAnalyticsTest } from './blockAnalytics';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { coverageTest } from './coverage';
import { currentsTest } from './currents';
import { databaseTest } from './db';
import { tokenRefreshTest } from './tokenRefresh';
import { unusedRepoUrlTest } from './unusedRepoUrl';

export const test = mergeTests(
  currentsTest,
  blockAnalyticsTest,
  clientTest,
  cleanupTest,
  coverageTest,
  databaseTest,
  tokenRefreshTest,
  unusedRepoUrlTest,
);

export { expect } from '@playwright/test';
