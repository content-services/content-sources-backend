import { expect, type Page } from '@playwright/test';

/** Default Patch API page size for template systems list; tests that assign one host do not need a higher cap. */
export const PATCH_TEMPLATE_SYSTEMS_PAGE_LIMIT = 100;

type PatchIdsRow = { id: string; attributes: { display_name: string } };
type PatchListRow = { attributes: { display_name: string }; inventory_id: string };

/**
 * Resolve Patch system id(s) for DELETE /templates/systems: prefer ids endpoint (SystemItem.id),
 * fall back to list endpoint (inventory_id) when ids returns 404 or no matching row.
 * Non-404, non-200 responses throw so unexpected API behavior is visible.
 */
export async function resolvePatchSystemIdsForHostname(
  page: Page,
  templateUuid: string,
  hostname: string,
): Promise<string[]> {
  const idsRes = await page.request.get(`/api/patch/v3/ids/templates/${templateUuid}/systems`);
  if (idsRes.status() === 200) {
    const idsBody = await idsRes.json();
    const byId = (idsBody.data as PatchIdsRow[])?.find(
      (sys) => sys.attributes?.display_name === hostname,
    );
    if (byId?.id) return [byId.id];
  } else if (idsRes.status() !== 404) {
    const body = await idsRes.text();
    throw new Error(
      `unexpected Patch GET ids/templates/.../systems status ${idsRes.status()}: ${body}`,
    );
  }

  const listRes = await page.request.get(
    `/api/patch/v3/templates/${templateUuid}/systems?limit=${PATCH_TEMPLATE_SYSTEMS_PAGE_LIMIT}&offset=0`,
  );
  if (listRes.status() === 200) {
    const listBody = await listRes.json();
    const row = (listBody.data as PatchListRow[])?.find(
      (sys) => sys.attributes?.display_name === hostname,
    );
    if (row?.inventory_id) return [row.inventory_id];
  } else if (listRes.status() !== 404) {
    const body = await listRes.text();
    throw new Error(
      `unexpected Patch GET templates/.../systems status ${listRes.status()}: ${body}`,
    );
  }

  return [];
}

/**
 * Assert the host no longer appears on the template’s systems list (GET ids endpoint).
 * 404 means no systems on the template, which satisfies the assertion.
 */
export async function expectHostnameAbsentFromPatchTemplate(
  page: Page,
  templateUuid: string,
  hostname: string,
): Promise<void> {
  const after = await page.request.get(`/api/patch/v3/ids/templates/${templateUuid}/systems`);
  if (after.status() === 404) {
    return;
  }
  expect(after.ok(), 'fetching template systems after removal should succeed').toBeTruthy();
  const afterBody = await after.json();
  const stillThere =
    (afterBody.data as Array<{ attributes?: { display_name?: string } }>)?.filter(
      (sys) => sys.attributes?.display_name === hostname,
    ) ?? [];
  expect(
    stillThere,
    'system should no longer be listed on the template in Patch after removal',
  ).toHaveLength(0);
}
