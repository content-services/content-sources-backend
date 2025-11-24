import { test as base, type TestInfo } from '@playwright/test';
import { ensureValidToken, fileNameToEnvVar } from '../helpers/tokenHelpers';

/**
 * Map storage state files to their corresponding auth file names for token refresh
 */
const USER_TOKEN_MAP = {
  '.auth/admin_user.json': 'admin_user.json',
  '.auth/stable_sam.json': 'stable_sam.json',
  '.auth/no_subs_user.json': 'no_subs_user.json',
  '.auth/layered-repo-user.json': 'layered-repo-user.json',
  '.auth/rhel-only-user.json': 'rhel-only-user.json',
  '.auth/read-only.json': 'read-only.json',
  '.auth/rhel_operator.json': 'rhel_operator.json',
} as const;

type UserAuthFile = keyof typeof USER_TOKEN_MAP;

/**
 * Get the auth file being used by the current test context
 */
function getAuthFileFromContext(testInfo: TestInfo): string | null {
  const storageState = testInfo.project?.use?.storageState;
  if (storageState && typeof storageState === 'string') {
    return storageState;
  }
  return null;
}

/**
 * Autofixture that automatically refreshes JWT tokens before each test
 * based on the storageState configured for that test.
 *
 * This fixture:
 * - Automatically detects which user token a test needs
 * - Checks if token is expiring (< 5 minutes remaining)
 * - Refreshes token only when needed
 * - Updates the correct environment variable
 * - Requires no changes to individual test files
 *
 * Usage in tests: Just import test from test-utils as normal
 * ```typescript
 * import { test, expect } from 'test-utils';
 *
 * test.use({ storageState: '.auth/stable_sam.json' });
 *
 * test('My test', async ({ client }) => {
 *   // Token is automatically refreshed if needed
 * });
 * ```
 */
export const tokenRefreshTest = base.extend({
  page: async ({ page }, use, testInfo) => {
    const authFile = getAuthFileFromContext(testInfo);

    if (authFile && authFile in USER_TOKEN_MAP) {
      const fileName = USER_TOKEN_MAP[authFile as UserAuthFile];
      const envVar = fileNameToEnvVar[fileName];
      const testName = `${testInfo.titlePath.join(' > ')}`;

      console.log(`\n[Token Refresh] Test: ${testName}`);
      console.log(`[Token Refresh] User: ${fileName}`);

      try {
        const refreshedToken = await ensureValidToken(page, fileName, 5);

        if (refreshedToken && envVar) {
          process.env[envVar] = refreshedToken;
        }
      } catch (error) {
        console.error(`[Token Refresh] Failed to refresh token for ${fileName}:`, error);
      }
    } else if (authFile) {
      console.log(`[Token Refresh] Unknown auth file: ${authFile} - skipping token refresh`);
    } else {
      console.log('[Token Refresh] No storageState configured - using default admin_user');
      try {
        const refreshedToken = await ensureValidToken(page, 'admin_user.json', 5);
        if (refreshedToken) {
          process.env.TOKEN = refreshedToken;
        }
      } catch (error) {
        console.error('[Token Refresh] Failed to refresh default token:', error);
      }
    }

    await use(page);
  },
});
