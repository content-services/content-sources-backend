import { test as oldTest } from '@playwright/test';

type WithCleanup = {
  cleanup: Cleanup;
};

export interface Cleanup {
  add: (cleanupFn: () => Promise<unknown>) => symbol;
  runAndAdd: (cleanupFn: () => Promise<unknown>) => Promise<symbol>;
  remove: (key: symbol) => void;
}

export const cleanupTest = oldTest.extend<WithCleanup>({
  // eslint-disable-next-line no-empty-pattern
  cleanup: async ({}, use) => {
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

    await cleanupTest.step(
      'Post-test cleanup',
      async () => {
        await Promise.all(Array.from(cleanupFns).map(([, fn]) => fn()));
      },
      { box: true },
    );
  },
});
