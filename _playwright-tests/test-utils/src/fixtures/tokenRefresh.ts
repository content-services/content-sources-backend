import { test as base, type TestInfo } from '@playwright/test';
import { ensureValidToken, fileNameToEnvVar, getFileNameFromAuthPath } from '../helpers/tokenHelpers';

/**
 * Get the storageState auth file path from test info
 */
function getAuthFileFromContext(testInfo: TestInfo): string | null {
  const project = testInfo.project as { use?: { storageState?: string } };
  const storageState = project?.use?.storageState;
  if (storageState && typeof storageState === 'string') {
    return storageState;
  }
  return null;
}

/**
 * Autofixture that automatically refreshes JWT tokens before each test
 * based on the storageState configured for that test.
 */
export const tokenRefreshTest = base.extend({
  page: async ({ page }, use, testInfo) => {
    const authFile = getAuthFileFromContext(testInfo);

    if (authFile) {
      const fileName = getFileNameFromAuthPath(authFile);
      const envVar = fileNameToEnvVar(fileName);

      try {
        const refreshedToken = await ensureValidToken(page, fileName, 5);

        if (refreshedToken) {
          process.env[envVar] = refreshedToken;
          console.log(`[Token Refresh] Refreshed ${envVar}`);
        }
      } catch (error) {
        console.error(`[Token Refresh] Failed to refresh token for ${fileName}:`, error);
      }
    } else {
      try {
        const refreshedToken = await ensureValidToken(page, 'ADMIN_TOKEN.json', 5);
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
