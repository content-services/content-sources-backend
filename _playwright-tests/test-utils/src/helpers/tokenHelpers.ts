import { type Page } from '@playwright/test';
import path from 'path';
import { readFileSync, existsSync } from 'fs';

/**
 * Directory for Playwright storageState JSON (<user_token>.json).
 * The consuming app sets PLAYWRIGHT_AUTH_DIR in playwright.config (frontend: repo-root `.auth`).
 * Fallback is for backend-only runs where `_playwright-tests/test-utils` layout matches `../../../.auth`.
 */
function resolveAuthStorageRoot(): string {
  const fromEnv = process.env.PLAYWRIGHT_AUTH_DIR;
  if (fromEnv) {
    return fromEnv;
  }
  return path.join(__dirname, '../../../.auth');
}

interface JWTPayload {
  exp: number;
  iat: number;
  [key: string]: unknown;
}

export interface TokenExpiryInfo {
  isExpired: boolean;
  isExpiringSoon: boolean;
  expiresAt: Date;
  timeRemainingMs: number;
  timeRemainingMinutes: number;
}

/**
 * Cut suffix from auth file name to get environment variable name
 * e.g., "ADMIN_TOKEN.json" → "ADMIN_TOKEN"
 */
export function fileNameToEnvVar(fileName: string): string {
  return fileName.replace('.json', '');
}

/**
 * Extract the file name from an auth file path
 * e.g., ".auth/ADMIN_TOKEN.json" → "ADMIN_TOKEN.json"
 */
export function getFileNameFromAuthPath(authPath: string): string {
  return authPath.replace('.auth/', '');
}

/**
 * Decode a JWT token and extract its payload
 */
export function decodeJWT(token: string): JWTPayload | null {
  try {
    const cleanToken = token.replace(/^Bearer\s+/i, '');
    const parts = cleanToken.split('.');
    if (parts.length !== 3) {
      return null;
    }

    const payload = parts[1];
    const base64 = payload.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = Buffer.from(base64, 'base64').toString('utf-8');

    return JSON.parse(jsonPayload) as JWTPayload;
  } catch {
    return null;
  }
}

/**
 * Extract token from a Playwright storageState JSON file.
 * Supports both cookies (cs_jwt) and localStorage (cs_jwt).
 */
function getTokenFromStorageStateFile(filePath: string): string | null {
  try {
    const absolutePath = path.isAbsolute(filePath)
      ? filePath
      : existsSync(filePath)
        ? path.resolve(filePath)
        : path.join(resolveAuthStorageRoot(), path.basename(filePath));

    if (!existsSync(absolutePath)) {
      return null;
    }

    const storage = JSON.parse(readFileSync(absolutePath, 'utf-8'));

    // Playwright storageState: origins -> localStorage
    const tokenItem = storage.origins?.[0]?.localStorage?.find(
      (i: { name: string }) => i.name === 'cs_jwt',
    );
    if (tokenItem?.value) {
      const val = tokenItem.value;
      return val.startsWith('Bearer ') ? val : `Bearer ${val}`;
    }

    // Fallback: cookies (cs_jwt)
    const cookies = storage.cookies ?? storage.origins?.[0]?.cookies;
    const jwtCookie = Array.isArray(cookies)
      ? cookies.find((c: { name: string }) => c.name === 'cs_jwt')
      : null;
    if (jwtCookie?.value) {
      return `Bearer ${jwtCookie.value}`;
    }

    return null;
  } catch {
    return null;
  }
}

/**
 * Get the stored JWT token from an auth state file
 * @param fileName - The auth state file name (e.g., 'ADMIN_TOKEN.json')
 * @returns The token with Bearer prefix, or null if not found
 */
export function getStoredToken(fileName: string): string | null {
  const authFilePath = path.join(resolveAuthStorageRoot(), fileName);
  return getTokenFromStorageStateFile(authFilePath);
}

/**
 * Check if a JWT token is expired or will expire soon
 * @param token - The JWT token (with or without "Bearer " prefix)
 * @param bufferMinutes - Minutes before expiry to consider "expiring soon" (default: 5)
 */
export function checkTokenExpiry(token: string, bufferMinutes: number = 5): TokenExpiryInfo {
  const payload = decodeJWT(token);

  if (!payload || !payload.exp) {
    throw new Error('Invalid JWT token or missing exp claim');
  }

  const expiresAtMs = payload.exp * 1000;
  const timeRemainingMs = expiresAtMs - Date.now();
  const bufferMs = bufferMinutes * 60 * 1000;

  return {
    isExpired: timeRemainingMs <= 0,
    isExpiringSoon: timeRemainingMs > 0 && timeRemainingMs <= bufferMs,
    expiresAt: new Date(expiresAtMs),
    timeRemainingMs,
    timeRemainingMinutes: Math.max(0, timeRemainingMs / 1000 / 60),
  };
}

/**
 * Refresh the JWT token by navigating to the app and extracting new token from cookies
 * @param page - Playwright page object
 * @param fileName - Name of the auth state file (e.g., 'ADMIN_TOKEN.json')
 */
export async function refreshJWTToken(page: Page, fileName: string): Promise<string> {
  await page.goto('/insights/content/repositories');
  await page.waitForLoadState('networkidle', { timeout: 30000 });

  const cookies = await page.context().cookies();
  const jwtCookie = cookies.find((cookie) => cookie.name === 'cs_jwt');

  if (!jwtCookie?.value) {
    throw new Error('Failed to refresh JWT token - cs_jwt cookie not found');
  }

  const newToken = `Bearer ${jwtCookie.value}`;

  // Save the updated storage state
  await page.context().storageState({
    path: path.join(resolveAuthStorageRoot(), fileName),
  });

  const envVarName = fileNameToEnvVar(fileName);
  process.env[envVarName] = newToken;

  return newToken;
}

const BUFFER_MINUTES = 5;

/**
 * Validate token (expiry check) and refresh via page if needed.
 * Retries refresh up to `retries` times on failure.
 */
async function validateWithBackend(
  page: Page,
  token: string,
  fileName: string,
  retries: number,
): Promise<string | null> {
  try {
    const expiry = checkTokenExpiry(token, BUFFER_MINUTES);
    if (!expiry.isExpired && !expiry.isExpiringSoon) {
      return null; // Token valid, no refresh needed
    }
  } catch {
    // Token invalid or malformed, need to refresh
  }

  let lastError: Error | null = null;
  for (let i = 0; i <= retries; i++) {
    try {
      return await refreshJWTToken(page, fileName);
    } catch (err) {
      lastError = err instanceof Error ? err : new Error(String(err));
    }
  }
  throw lastError ?? new Error('Token validation failed');
}

/**
 * Validates a token with the backend.
 * @param page - The Playwright page object
 * @param tokenOrPath - Can be a raw token, a .json file path, or undefined (defaults to ADMIN_TOKEN)
 * @param retries - Number of retries for refresh on validation failure (default: 5)
 * @returns The refreshed token if it was refreshed, null if no refresh was needed
 */
export async function ensureValidToken(
  page: Page,
  tokenOrPath?: string,
  retries: number = 5,
): Promise<string | null> {
  let token: string | undefined;

  // 1. Check if we were given a path to a Playwright storageState JSON
  if (tokenOrPath?.endsWith('.json')) {
    token = getTokenFromStorageStateFile(tokenOrPath) ?? undefined;
    if (!token) {
      console.error(`Failed to read token from file ${tokenOrPath}`);
    }
  }
  // 2. If it's a string but not a JSON file, treat it as a raw token
  else if (tokenOrPath) {
    token = tokenOrPath.startsWith('Bearer ') ? tokenOrPath : `Bearer ${tokenOrPath}`;
  }

  // 3. Fallback: try env var, then page cookies, then storage file
  if (!token) {
    token = process.env.ADMIN_TOKEN;
  }
  if (!token) {
    const cookies = await page.context().cookies();
    const jwtCookie = cookies.find((cookie) => cookie.name === 'cs_jwt');
    token = jwtCookie ? `Bearer ${jwtCookie.value}` : undefined;
  }
  if (!token) {
    token = getStoredToken('ADMIN_TOKEN.json') ?? undefined;
  }

  if (!token) {
    throw new Error('No valid token found for authentication.');
  }

  const fileName = tokenOrPath?.endsWith('.json')
    ? path.basename(tokenOrPath)
    : 'ADMIN_TOKEN.json';

  return validateWithBackend(page, token, fileName, retries);
}

/**
 * Get token expiry information for a specific user from their auth state file
 * @param fileName - The auth state file name (e.g., 'ADMIN_TOKEN.json')
 */
export function getStoredTokenExpiry(fileName: string): TokenExpiryInfo | null {
  const token = getStoredToken(fileName);
  if (!token) {
    return null;
  }

  try {
    return checkTokenExpiry(token);
  } catch {
    return null;
  }
}
