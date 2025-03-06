import {
  BulkDeleteRepositoriesRequest,
  Configuration,
  DeleteTemplateRequest,
  ListTemplatesRequest,
  RepositoriesApi,
  TemplatesApi,
  type ListRepositoriesRequest,
} from './client';

/* eslint-disable @typescript-eslint/no-explicit-any */
// while condition is true, calls fn, waits interval (ms) between calls.
// condition's parameter should be the result of the function call.
export const poll = async (
  fn: () => Promise<any>,
  condition: (result: any) => boolean,
  interval: number,
) => {
  let result = await fn();
  while (condition(result)) {
    result = await fn();
    await timer(interval);
  }
  return result;
};

// timer in ms, must await e.g. await timer(num)
export const timer = (ms: number) => new Promise((res) => setTimeout(res, ms));

export const cleanupRepositories = async (
  client: Configuration,
  repoNames: string[],
  urls: string[],
) => {
  let uuidList: string[] = [];
  for (const s of repoNames.concat(urls)) {
    const res = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
      origin: 'external,upload',
      search: s,
    });
    if (res.data?.length) {
      uuidList = uuidList.concat(res.data.map((v) => v.uuid!));
    }
  }

  if (uuidList.length) {
    await new RepositoriesApi(client).bulkDeleteRepositoriesRaw(<BulkDeleteRepositoriesRequest>{
      apiUUIDListRequest: { uuids: [...new Set(uuidList)] },
    });
  }
};

export const cleanupTemplates = async (client: Configuration, templateNames: string[]) => {
  let uuidList: string[] = [];
  for (const n of templateNames) {
    const res = await new TemplatesApi(client).listTemplates(<ListTemplatesRequest>{
      name: n,
    });
    if (res.data?.length) {
      uuidList = uuidList.concat(res.data.map((v) => v.uuid!));
    }
  }

  for (const u in uuidList) {
    await new TemplatesApi(client).deleteTemplateRaw(<DeleteTemplateRequest>{
      uuid: u,
    });
  }
};
