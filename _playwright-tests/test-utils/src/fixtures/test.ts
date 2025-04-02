import { mergeTests } from '@playwright/test';
import { cleanupTest } from './cleanup';
import { clientTest } from './client';
import { databaseTest } from './db';

export const test = mergeTests(clientTest, cleanupTest, databaseTest);
export { expect } from '@playwright/test';
