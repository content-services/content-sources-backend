import { expect, Page } from '@playwright/test';

/** Poll timeout for system propagation to inventory and patch (3 minutes) */
export const INVENTORY_PATCH_POLL_TIMEOUT_MS = 180_000;

/**
 * Delay between Inventory and Patch polling attempts. Playwright can emit a network line per
 * `page.request` call; a modest interval keeps logs readable without materially slowing success.
 */
const INVENTORY_PATCH_POLL_INTERVAL_MS = 10_000;

type PatchSystemAttributes = {
  display_name: string;
  template_uuid?: string | null;
};

type PatchSystemRecord = {
  attributes: PatchSystemAttributes;
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
 */
export const waitInPatch = async (
  page: Page,
  hostname: string,
  expectedAttachment: boolean = true,
  timeoutMs: number = INVENTORY_PATCH_POLL_TIMEOUT_MS,
): Promise<void> => {
  const pollOptions = {
    timeout: timeoutMs,
    intervals: [INVENTORY_PATCH_POLL_INTERVAL_MS] as [number],
  };

  await expect
    .poll(() => isInInventory(page, hostname), {
      ...pollOptions,
      message: 'System did not appear in inventory in time',
    })
    .toBe(1);

  if (expectedAttachment) {
    await expect
      .poll(() => isSystemListedInPatch(page, hostname), {
        ...pollOptions,
        message: 'System did not appear in Patch in time',
      })
      .toBe(1);

    await expect
      .poll(() => hasTemplateUuidInPatch(page, hostname), {
        ...pollOptions,
        message:
          'System is in Patch but template_uuid is not set (content-template facts may not be uploaded yet)',
      })
      .toBe(1);
    return;
  }

  await expect
    .poll(() => isInPatch(page, hostname, false), {
      ...pollOptions,
      message: 'System did not appear in Patch without a template in time',
    })
    .toBe(1);
};
