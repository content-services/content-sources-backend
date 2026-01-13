import { type Page } from '@playwright/test';
import path from 'path';
import { readFileSync, existsSync } from 'fs';

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
 * Get the stored JWT token from an auth state file
 * @param fileName - The auth state file name (e.g., 'ADMIN_TOKEN.json')
 * @returns The token with Bearer prefix, or null if not found
 */
export function getStoredToken(fileName: string): string | null {
  try {
    const authFilePath = path.join(__dirname, '../../../.auth', fileName);
    if (!existsSync(authFilePath)) {
      return null;
    }

    const authState: AuthState = JSON.parse(readFileSync(authFilePath, 'utf-8'));
    const jwtCookie = authState.cookies?.find((c) => c.name === 'cs_jwt');

    if (!jwtCookie?.value) {
      return null;
    }

    return `Bearer ${jwtCookie.value}`;
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
    path: path.join(__dirname, '../../../.auth', fileName),
  });

  const envVarName = fileNameToEnvVar(fileName);
  process.env[envVarName] = newToken;

  return newToken;
}

/**
 * Check if token needs refresh and refresh it if necessary
 * @param page - Playwright page object
 * @param fileName - Auth state filename
 * @param bufferMinutes - Refresh if expiring within this many minutes (default: 5)
 * @returns The refreshed token if it was refreshed, null if no refresh was needed
 */
export async function ensureValidToken(
  page: Page,
  fileName: string,
  bufferMinutes: number = 5,
): Promise<string | null> {
  const envVarName = fileNameToEnvVar(fileName);
  let currentToken = process.env[envVarName];

  // If no token in env, try to get it from cookies
  if (!currentToken) {
    const cookies = await page.context().cookies();
    const jwtCookie = cookies.find((cookie) => cookie.name === 'cs_jwt');
    currentToken = jwtCookie ? `Bearer ${jwtCookie.value}` : '';
  }

  // If still no token, try to read from auth state file
  if (!currentToken) {
    currentToken = getStoredToken(fileName) ?? '';
  }

  if (!currentToken) {
    // No token found anywhere, nothing to validate
    return null;
  }

  try {
    const expiry = checkTokenExpiry(currentToken, bufferMinutes);

    if (expiry.isExpired || expiry.isExpiringSoon) {
      return await refreshJWTToken(page, fileName);
    }

    // Token is still valid, no refresh needed
    return null;
  } catch {
    // Token is invalid, refresh it
    return await refreshJWTToken(page, fileName);
  }
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
