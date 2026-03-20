import { test as oldTest, type TestInfo } from '@playwright/test';
import { ensureValidToken, fileNameToEnvVar, getFileNameFromAuthPath } from '../helpers/tokenHelpers';

type WithCleanup = {
  cleanup: Cleanup;
};

export interface Cleanup {
  add: (cleanupFn: () => Promise<unknown>) => symbol;
  runAndAdd: (cleanupFn: () => Promise<unknown>) => Promise<symbol>;
  remove: (key: symbol) => void;
}

function getAuthFileFromContext(testInfo: TestInfo): string | null {
  const project = testInfo.project as { use?: { storageState?: string } };
  const storageState = project?.use?.storageState;
  if (storageState && typeof storageState === 'string') {
    return storageState;
  }
  return null;
}

export const cleanupTest = oldTest.extend<WithCleanup>({
  cleanup: async ({ page }, use, testInfo) => {
    const cleanupFns: Map<symbol, () => Promise<unknown>> = new Map();

    await use({
      add: (cleanupFn) => {
        const key = Symbol();
        cleanupFns.set(key, cleanupFn);
        return key;
      },
      runAndAdd: async (cleanupFn) => {
        await cleanupFn();

        const key = Symbol();
        cleanupFns.set(key, cleanupFn);
        return key;
      },
      remove: (key) => {
        cleanupFns.delete(key);
      },
    });

    const authFile = getAuthFileFromContext(testInfo);
    const fileName = authFile ? getFileNameFromAuthPath(authFile) : 'ADMIN_TOKEN.json';
    try {
      const refreshedToken = await ensureValidToken(page, fileName, 5);
      if (refreshedToken) {
        process.env[fileNameToEnvVar(fileName)] = refreshedToken;
      }
    } catch (error) {
      console.error('[Cleanup] Failed to ensure valid token before cleanup:', error);
    }

    await cleanupTest.step(
      'Post-test cleanup',
      async () => {
        await Promise.all(Array.from(cleanupFns).map(([, fn]) => fn()));
      },
      { box: true },
    );
  },
});
