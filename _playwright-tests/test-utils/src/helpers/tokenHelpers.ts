import { type Page } from '@playwright/test';
import path from 'path';
import { readFileSync, existsSync } from 'fs';

export const fileNameToEnvVar: Record<string, string> = {
  'admin_user.json': 'TOKEN',
  'rhel_operator.json': 'RHEL_OPERATOR_TOKEN',
  'read-only.json': 'READONLY_USER_TOKEN',
  'layered-repo-user.json': 'LAYERED_REPO_USER_TOKEN',
  'rhel-only-user.json': 'RHEL_ONLY_USER_TOKEN',
  'no_subs_user.json': 'NO_SUBS_USER_TOKEN',
  'stable_sam.json': 'STABLE_SAM_TOKEN',
};

interface JWTPayload {
  exp: number;
  iat: number;
  [key: string]: unknown;
}

interface AuthCookie {
  name: string;
  value: string;
}

interface AuthState {
  cookies?: AuthCookie[];
}

export interface TokenExpiryInfo {
  isExpired: boolean;
  isExpiringSoon: boolean;
  expiresAt: Date;
  timeRemainingMs: number;
  timeRemainingMinutes: number;
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
 * @param fileName - Name of the auth state file (e.g., 'admin_user.json')
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

  await page.context().storageState({
    path: path.join(__dirname, '../../../.auth', fileName),
  });

  const envVarName = fileNameToEnvVar[fileName] || 'TOKEN';
  process.env[envVarName] = newToken;

  return newToken;
}

/**
 * Check if token needs refresh and refresh it if necessary
 * @param page - Playwright page object
 * @param fileName - Auth state filename
 * @param bufferMinutes - Refresh if expiring within this many minutes (default: 5)
 */
export async function ensureValidToken(
  page: Page,
  fileName: string = 'admin_user.json',
  bufferMinutes: number = 5,
): Promise<string> {
  const envVarName = fileNameToEnvVar[fileName] || 'TOKEN';
  let currentToken = process.env[envVarName];

  if (!currentToken) {
    const cookies = await page.context().cookies();
    const jwtCookie = cookies.find((cookie) => cookie.name === 'cs_jwt');
    currentToken = jwtCookie ? `Bearer ${jwtCookie.value}` : '';
  }

  if (!currentToken) {
    return '';
  }

  try {
    const expiry = checkTokenExpiry(currentToken, bufferMinutes);

    if (expiry.isExpired || expiry.isExpiringSoon) {
      return await refreshJWTToken(page, fileName);
    }

    return currentToken;
  } catch {
    return await refreshJWTToken(page, fileName);
  }
}

/**
 * Get token expiry information for a specific user from their auth state file
 * @param userName - The user type ('admin_user', 'no_subs_user', 'stable_sam', etc.)
 */
export function getStoredTokenExpiry(userName: string): TokenExpiryInfo | null {
  try {
    const authPath = path.join(__dirname, `../../../.auth/${userName}.json`);

    if (!existsSync(authPath)) {
      return null;
    }

    const authStateContent = readFileSync(authPath, 'utf-8');
    const authState: AuthState = JSON.parse(authStateContent);
    const jwtCookie = authState.cookies?.find((c) => c.name === 'cs_jwt');

    if (!jwtCookie) {
      return null;
    }

    return checkTokenExpiry(`Bearer ${jwtCookie.value}`);
  } catch {
    return null;
  }
}
