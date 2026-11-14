import { expect, Page } from '@playwright/test';
import { ensureValidToken, usesIdentityHeaderAuth } from './tokenHelpers';

/** Poll timeout for system propagation to inventory and patch (20 seconds) */
export const INVENTORY_PATCH_POLL_TIMEOUT_MS = 20_000;

/**
 * Delay between Inventory and Patch polling attempts. Kept short relative to the
 * overall timeout so failures still gather several samples before dumping debug.
 */
const INVENTORY_PATCH_POLL_INTERVAL_MS = 2_000;

type PatchSystemAttributes = {
  display_name: string;
  template_uuid?: string | null;
};

type PatchSystemRecord = {
  attributes: PatchSystemAttributes;
};

type InventoryHostRecord = {
  id?: string;
  display_name?: string;
  fqdn?: string;
  insights_id?: string;
  reporter?: string;
};

/** Result of looking up a host in the Patch systems API by display name. */
export type PatchSystemLookupResult =
  | { kind: 'api_error' }
  | { kind: 'not_found' }
  | { kind: 'found'; templateUuid: string | null };

/**
 * Look up a system in Patch by hostname (display_name).
 */
export async function getPatchSystemByHostname(
  page: Page,
  hostname: string,
): Promise<PatchSystemLookupResult> {
  try {
    const response = await page.request.get(
      `/api/patch/v3/systems?search=${encodeURIComponent(hostname)}&limit=100`,
    );

    if (response.status() !== 200) {
      console.log(`⚠️  API request failed with status ${response.status()}`);
      return { kind: 'api_error' };
    }

    const body = await response.json();
    const system = (body.data as PatchSystemRecord[] | undefined)?.find(
      (sys) => sys.attributes.display_name === hostname,
    );

    if (!system) {
      return { kind: 'not_found' };
    }

    const templateUuid = system.attributes?.template_uuid ?? null;
    return {
      kind: 'found',
      templateUuid: templateUuid && String(templateUuid).length > 0 ? String(templateUuid) : null,
    };
  } catch (error) {
    console.log('⚠️  Error checking system in patch:', error);
    return { kind: 'api_error' };
  }
}

/**
 * Dump Inventory and Patch API payloads once after a waitInPatch poll fails.
 * Helps distinguish hostname mismatch, empty search hits, and API errors.
 */
export async function logWaitInPatchDebug(
  page: Page,
  hostname: string,
  reason: string,
): Promise<void> {
  console.log(`\n=== waitInPatch debug (${reason}) hostname="${hostname}" ===`);

  try {
    const inventoryUrl = `/api/inventory/v1/hosts?display_name=${encodeURIComponent(hostname)}`;
    const inventoryResponse = await page.request.get(inventoryUrl);
    console.log(`Inventory GET ${inventoryUrl} -> ${inventoryResponse.status()}`);

    if (inventoryResponse.status() === 200) {
      const body = await inventoryResponse.json();
      const results = (body.results as InventoryHostRecord[] | undefined) ?? [];
      console.log(`Inventory match count: ${results.length}`);
      console.log(
        'Inventory hosts:',
        JSON.stringify(
          results.map((host) => ({
            id: host.id,
            display_name: host.display_name,
            fqdn: host.fqdn,
            insights_id: host.insights_id,
            reporter: host.reporter,
          })),
          null,
          2,
        ),
      );
    } else {
      console.log('Inventory body:', await inventoryResponse.text());
    }
  } catch (error) {
    console.log('⚠️  Failed to fetch Inventory debug payload:', error);
  }

  try {
    const patchUrl = `/api/patch/v3/systems?search=${encodeURIComponent(hostname)}&limit=100`;
    const patchResponse = await page.request.get(patchUrl);
    console.log(`Patch GET ${patchUrl} -> ${patchResponse.status()}`);

    if (patchResponse.status() === 200) {
      const body = await patchResponse.json();
      const systems = (body.data as PatchSystemRecord[] | undefined) ?? [];
      const exactMatch = systems.find((sys) => sys.attributes.display_name === hostname);
      console.log(`Patch search hit count: ${systems.length}`);
      console.log(
        'Patch display_names:',
        JSON.stringify(
          systems.map((sys) => ({
            display_name: sys.attributes.display_name,
            template_uuid: sys.attributes.template_uuid ?? null,
          })),
          null,
          2,
        ),
      );
      console.log(
        `Exact display_name match: ${exactMatch ? 'yes' : 'no'}` +
          (exactMatch
            ? ` (template_uuid=${exactMatch.attributes.template_uuid ?? null})`
            : ''),
      );
    } else {
      console.log('Patch body:', await patchResponse.text());
    }
  } catch (error) {
    console.log('⚠️  Failed to fetch Patch debug payload:', error);
  }

  console.log('=== end waitInPatch debug ===\n');
}

/**
 * @returns 1 if the host exists in Patch, 0 if not found, -1 on API error
 */
export const isSystemListedInPatch = async (page: Page, hostname: string): Promise<number> => {
  const result = await getPatchSystemByHostname(page, hostname);
  if (result.kind === 'api_error') return -1;
  if (result.kind === 'not_found') return 0;
  return 1;
};

/**
 * @returns 1 if the host is in Patch with template_uuid set, 0 if missing or host has no template, -1 on API error
 */
export const hasTemplateUuidInPatch = async (page: Page, hostname: string): Promise<number> => {
  const result = await getPatchSystemByHostname(page, hostname);
  if (result.kind === 'api_error') return -1;
  if (result.kind === 'not_found') return 0;
  if (result.templateUuid) return 1;
  console.log(
    `⚠️  System "${hostname}" is in Patch but template_uuid is not set yet (waiting for content-template facts upload)`,
  );
  return 0;
};

/**
 * Count matching systems in Patch.
 * @returns Promise<number> - number of matching systems, -1 on error
 */
export const isInPatch = async (
  page: Page,
  hostname: string,
  expectedAttachment: boolean = true,
): Promise<number> => {
  const result = await getPatchSystemByHostname(page, hostname);
  if (result.kind === 'api_error') return -1;
  if (result.kind === 'not_found') return 0;

  const hasTemplate = Boolean(result.templateUuid);
  if (hasTemplate === expectedAttachment) return 1;
  return 0;
};

/**
 * Count matching systems in Inventory.
 * @returns Promise<number> - number of matching systems, -1 on error
 */
export const isInInventory = async (page: Page, hostname: string): Promise<number> => {
  try {
    const response = await page.request.get(
      `/api/inventory/v1/hosts?display_name=${encodeURIComponent(hostname)}`,
    );

    if (response.status() !== 200) {
      console.log(`⚠️  API request failed with status ${response.status()}`);
      return -1;
    }

    const body = await response.json();
    return body.results?.length ?? 0;
  } catch (error) {
    console.log('⚠️  Error checking system in inventory:', error);
    return -1;
  }
};

/**
 * Get the number of systems assigned to a template using the Patch API.
 * @returns Promise<number> - total_items from meta data object, or 0 when 404 (no systems assigned),
 *   or -1 for retryable errors (502, 503, 504) so the poll continues
 * @throws Error when the API request fails with a non-retryable status (not 200, 404, 502, 503, 504)
 */
export const getTemplateSystemsCount = async (
  page: Page,
  templateUuid: string,
): Promise<number> => {
  try {
    const response = await page.request.get(
      `/api/patch/v3/templates/${templateUuid}/systems?limit=1&offset=0`,
    );

    if (response.status() === 404) {
      return 0;
    }

    // Retryable gateway/upstream errors - return -1 so poll continues (no per-iteration logging)
    if (response.status() === 502 || response.status() === 503 || response.status() === 504) {
      return -1;
    }

    if (response.status() !== 200) {
      throw new Error(`Patch template systems API failed with status ${response.status()}`);
    }

    const body = await response.json();
    return body.meta?.total_items ?? 0;
  } catch (error) {
    if (error instanceof Error && error.message.startsWith('Patch template systems API failed')) {
      throw error;
    }
    const messagePrefix = 'Error fetching template systems from Patch';
    throw new Error(`${messagePrefix}: ${error instanceof Error ? error.message : String(error)}`);
  }
};

/**
 * Wait for host to appear in Inventory and Patch.
 * When expecting a content template, polls for the host in Patch first, then until template_uuid is set.
 *
 * Refreshes the browser JWT on each poll iteration so long waits (up to timeoutMs) do not fail
 * after the token expires. No-op when the token is still valid, or when IDENTITY_HEADER auth is used.
 *
 * On poll failure, dumps Inventory and Patch API payloads once for debugging.
 *
 * @param authStateFile - Playwright storageState filename for ensureValidToken (default: ADMIN_TOKEN.json)
 */
export const waitInPatch = async (
  page: Page,
  hostname: string,
  expectedAttachment: boolean = true,
  timeoutMs: number = INVENTORY_PATCH_POLL_TIMEOUT_MS,
  authStateFile: string = 'ADMIN_TOKEN.json',
): Promise<void> => {
  const pollOptions = {
    timeout: timeoutMs,
    intervals: [INVENTORY_PATCH_POLL_INTERVAL_MS] as [number],
  };

  const withValidToken = async (check: () => Promise<number>): Promise<number> => {
    if (!usesIdentityHeaderAuth()) {
      await ensureValidToken(page, authStateFile);
    }
    return check();
  };

  const pollUntil = async (check: () => Promise<number>, message: string): Promise<void> => {
    try {
      await expect.poll(() => withValidToken(check), { ...pollOptions, message }).toBe(1);
    } catch (error) {
      await logWaitInPatchDebug(page, hostname, message);
      throw error;
    }
  };

  await pollUntil(() => isInInventory(page, hostname), 'System did not appear in inventory in time');

  if (expectedAttachment) {
    await pollUntil(
      () => isSystemListedInPatch(page, hostname),
      'System did not appear in Patch in time',
    );

    await pollUntil(
      () => hasTemplateUuidInPatch(page, hostname),
      'System is in Patch but template_uuid is not set (content-template facts might not be uploaded yet)',
    );
    return;
  }

  await pollUntil(
    () => isInPatch(page, hostname, false),
    'System did not appear in Patch without a template in time',
  );
};
