import { test } from '../fixtures';
import {
  ApiTaskInfoCollectionResponse,
  BulkDeleteRepositoriesRequest,
  Configuration,
  DeleteTemplateRequest,
  ListRepositoriesRequest,
  ListTasksRequest,
  ListTemplatesRequest,
  RepositoriesApi,
  TasksApi,
  TemplatesApi,
} from '../client';
import { poll, sleep } from './poll';

export const cleanupRepositories = async (client: Configuration, ...namesOrUrls: string[]) => {
  await test.step(
    `Cleaning up repositories, search terms: ${namesOrUrls.join(', ')}`,
    async () => {
      let uuidList: string[] = [];
      let snapshotReposList: string[] = [];
      for (const s of namesOrUrls) {
        const res = await new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
          origin: 'external,upload',
          search: s,
        });
        if (res.data?.length) {
          uuidList = uuidList.concat(res.data.map((v) => v.uuid!));
          snapshotReposList = snapshotReposList.concat(
            res.data.filter((r) => r.snapshot).map((r) => r.uuid!),
          );
        }
      }

      if (uuidList.length) {
        await new RepositoriesApi(client).bulkDeleteRepositoriesRaw(<BulkDeleteRepositoriesRequest>{
          apiUUIDListRequest: { uuids: [...new Set(uuidList)] },
        });
      } else {
        return;
      }

      if (snapshotReposList.length) {
        await sleep(1000);

        const waitForTasks = (resp: ApiTaskInfoCollectionResponse) =>
          resp.data?.filter((t) => t.status == 'completed').length !== snapshotReposList.length;
        const getTask = () =>
          new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'delete-repository-snapshots',
            limit: snapshotReposList.length,
          });
        await poll(getTask, waitForTasks, 100);
      }
    },
    {
      box: true,
    },
  );
};

export const cleanupTemplates = async (client: Configuration, ...templateNames: string[]) => {
  await test.step(
    `Cleaning up templates, names: ${templateNames.join(', ')}`,
    async () => {
      let uuidList: string[] = [];

      // Fetch all templates and filter by prefix in code
      const res = await new TemplatesApi(client).listTemplates(<ListTemplatesRequest>{
        limit: 1000, // Get a large number of templates
      });

      if (res.data?.length) {
        for (const n of templateNames) {
          const matchingTemplates = res.data.filter((template) => template.name?.startsWith(n));
          uuidList = uuidList.concat(matchingTemplates.map((v) => v.uuid!));
        }
      }

      for (const u of uuidList) {
        await new TemplatesApi(client).deleteTemplateRaw(<DeleteTemplateRequest>{
          uuid: u,
        });
      }

      if (uuidList.length > 0) {
        await sleep(1000);
        const waitForTasks = (resp: ApiTaskInfoCollectionResponse) =>
          resp.data?.filter((t) => t.status == 'completed').length !== uuidList.length;
        const getTask = () =>
          new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'delete-templates',
            limit: uuidList.length,
          });

        await poll(getTask, waitForTasks, 100);
      }
    },
    {
      box: true,
    },
  );
};
