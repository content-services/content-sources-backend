import { test as base } from '@playwright/test';
import {
  authFileFromStorageState,
  ensureValidToken,
  fileNameToEnvVar,
  getFileNameFromAuthPath,
  usesIdentityHeaderAuth,
} from '../helpers/tokenHelpers';

/**
 * Autofixture that automatically refreshes JWT tokens before each test
 * based on the effective storageState for that test (project + test.use).
 *
 * Important: depend on the Playwright `storageState` option fixture — not
 * `testInfo.project.use.storageState` — so per-test overrides like
 * `test.use({ storageState: '.auth/NO_SUBS_TOKEN.json' })` are honored.
 * Otherwise a non-admin page session can be written into ADMIN_TOKEN.json.
 */
export const tokenRefreshTest = base.extend({
  page: async ({ page, storageState }, use) => {
    if (usesIdentityHeaderAuth()) {
      await use(page);
      return;
    }

    const authFile = authFileFromStorageState(storageState);

    if (authFile) {
      const fileName = getFileNameFromAuthPath(authFile);
      const envVar = fileNameToEnvVar(fileName);

      try {
        const refreshedToken = await ensureValidToken(page, authFile, 5);

        if (refreshedToken) {
          process.env[envVar] = refreshedToken;
          console.log(`[Token Refresh] Refreshed ${envVar}`);
        }
      } catch (error) {
        console.error(`[Token Refresh] Failed to refresh token for ${fileName}:`, error);
      }
    } else {
      try {
        const refreshedToken = await ensureValidToken(page, undefined, 5);
        if (refreshedToken) {
          process.env.ADMIN_TOKEN = refreshedToken;
          console.log('[Token Refresh] Refreshed ADMIN_TOKEN');
        }
      } catch (error) {
        console.error('[Token Refresh] Failed to refresh default token:', error);
      }
    }

    await use(page);
  },
});
