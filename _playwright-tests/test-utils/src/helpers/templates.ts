import { Configuration, ListTemplatesRequest, TemplatesApi } from '../client';

const TEMPLATE_SEARCH_PAGE_SIZE = 100;

/**
 * Find template UUID by name using the content-sources API.
 * Pages through results to handle larger environments.
 * @returns The template UUID, or null if not found
 */
export async function getTemplateUuidByName(
  client: Configuration,
  templateName: string,
): Promise<string | null> {
  const api = new TemplatesApi(client);
  let offset = 0;

  while (true) {
    const res = await api.listTemplates(<ListTemplatesRequest>{
      limit: TEMPLATE_SEARCH_PAGE_SIZE,
      offset,
      search: templateName,
    });

    const match = res.data?.find((t) => t.name === templateName);
    if (match) return match.uuid ?? null;

    if ((res.data?.length ?? 0) < TEMPLATE_SEARCH_PAGE_SIZE) break;

    offset += TEMPLATE_SEARCH_PAGE_SIZE;
  }

  return null;
}
