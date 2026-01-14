import { mergeTests } from '@playwright/test';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { databaseTest } from './db';
import { unusedRepoUrlTest } from './unusedRepoUrl';

export const test = mergeTests(clientTest, cleanupTest, databaseTest, unusedRepoUrlTest);
export { expect } from '@playwright/test';
