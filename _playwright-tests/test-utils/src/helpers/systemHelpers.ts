import { Page, expect } from '@playwright/test';

/** Poll timeout for system propagation to inventory and patch (3 minutes) */
export const INVENTORY_PATCH_POLL_TIMEOUT_MS = 180_000;

/**
 * Count matching systems in Patch.
 * @returns Promise<number> - number of matching systems, -1 on error
 */
export const isInPatch = async (
  page: Page,
  hostname: string,
  expectedAttachment: boolean = true,
): Promise<number> => {
  try {
    const response = await page.request.get(
      `/api/patch/v3/systems?search=${encodeURIComponent(hostname)}&limit=100`,
    );

    if (response.status() !== 200) {
      console.log(`⚠️  API request failed with status ${response.status()}`);
      return -1;
    }

    const body = await response.json();
    const system = body.data?.find(
      (sys: { attributes: { display_name: string } }) => sys.attributes.display_name === hostname,
    );

    if (!system) return 0;

    const hasTemplate = !!system.attributes?.template_uuid;
    if (hasTemplate === expectedAttachment) return 1;
    return 0;
  } catch (error) {
    console.log('⚠️  Error checking system in patch:', error);
    return -1;
  }
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
 * Wait for host to appear in Inventory and Patch
 */
export const waitInPatch = async (
  page: Page,
  hostname: string,
  expectedAttachment: boolean = true,
  timeoutMs: number = INVENTORY_PATCH_POLL_TIMEOUT_MS,
): Promise<void> => {
  await expect
    .poll(async () => await isInInventory(page, hostname), {
      message: 'System did not appear in inventory in time',
      timeout: timeoutMs,
    })
    .toBe(1);

  await expect
    .poll(async () => await isInPatch(page, hostname, expectedAttachment), {
      message: 'System did not appear in patch in time',
      timeout: timeoutMs,
    })
    .toBe(1);
};
