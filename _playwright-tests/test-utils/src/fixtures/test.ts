import { mergeTests } from '@playwright/test';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { databaseTest } from './db';
import { tokenRefreshTest } from './tokenRefresh';

export const test = mergeTests(clientTest, cleanupTest, databaseTest, tokenRefreshTest);
export { expect } from '@playwright/test';
