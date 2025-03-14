// Define a fixture to hold the API client
import { test as oldTest, expect } from '@playwright/test';
import { Configuration, ResponseContext, ResponseError } from './client';
import { setGlobalDispatcher, ProxyAgent } from 'undici';
import { cleanupRepositories, cleanupTemplates } from './helpers/apiHelpers';

type WithApiConfig = {
  client: Configuration;
  cleanup: Cleanup;
};

export interface Cleanup {
  add: (cleanupFn: () => Promise<unknown>) => symbol;
  remove: (key: symbol) => void;
}

export type CleanupConfig = {
  repoNames?: string[];
  urls?: string[];
  templateNames?: string[];
};

export type Annotations = Array<{
  type: string;
  description?: string;
}>;

// Default error handling doesn't print the error, so print it here
const responseReader = {
  post: async function (context: ResponseContext): Promise<void> {
    if (context.response != undefined && context.response.status > 300) {
      const bodyText = await context.response.text();
      console.log('Response errored with ' + context.response.status + ': ' + bodyText);
    }
  },
};

const mapAnnotationsToConfig = (annotations: Annotations): CleanupConfig => {
  const repos = annotations.find((v) => v.type == 'repoNames')?.description?.split(';') ?? [];
  const urls = annotations.find((v) => v.type == 'urls')?.description?.split(';') ?? [];
  const templates =
    annotations.find((v) => v.type == 'templateNames')?.description?.split(';') ?? [];
  return { repoNames: repos, urls: urls, templateNames: templates };
};

export const confToAnnotations = (conf: CleanupConfig): Annotations => {
  let annts: Annotations = [];
  annts = annts.concat({ type: 'repoNames', description: conf.repoNames?.join(';') });
  annts = annts.concat({ type: 'urls', description: conf.urls?.join(';') });
  annts = annts.concat({ type: 'templateNames', description: conf.templateNames?.join(';') });
  return annts;
};

const cleanup = async (client: Configuration, config: CleanupConfig, title: string) => {
  await test.step(
    title,
    async () => {
      if (config.repoNames?.length || config.urls?.length) {
        // expect(false).toBeTruthy();
        await cleanupRepositories(client, config.repoNames!, config.urls!);
      }
      if (config.templateNames?.length) {
        await cleanupTemplates(client, config.templateNames!);
      }
    },
    { box: true },
  );
};

export const test = oldTest.extend<WithApiConfig>({
  client:
    // eslint-disable-next-line no-empty-pattern
    async ({}, use, r) => {
      if (r.project?.use?.proxy?.server) {
        const dispatcher = new ProxyAgent({ uri: new URL(r.project.use.proxy.server).toString() });
        setGlobalDispatcher(dispatcher);
      }

      const client = new Configuration({
        basePath: r.project.use.baseURL + '/api/content-sources/v1',
        headers: r.project.use.extraHTTPHeaders,
        middleware: [responseReader],
      });

      const cleanupConfig: CleanupConfig = mapAnnotationsToConfig(r.annotations);
      await cleanup(client, cleanupConfig, 'Pre test cleanup');

      await use(client);

      await cleanup(client, cleanupConfig, 'Post test cleanup');
    },
  // eslint-disable-next-line no-empty-pattern
  cleanup: async ({}, use) => {
    const cleanupFns: Map<symbol, () => Promise<unknown>> = new Map();

    await use({
      add: (cleanupFn) => {
        const key = Symbol();
        cleanupFns.set(key, cleanupFn);
        return key;
      },
      remove: (key) => {
        cleanupFns.delete(key);
      },
    });

    await Promise.all(Array.from(cleanupFns).map(([, fn]) => fn()));
  },
});

export async function expectErrorStatus<T>(responseCode: number, apiCall: Promise<T>) {
  await expectError(responseCode, '', apiCall);
}

export async function expectError<T>(
  responseCode: number,
  bodyContains: string,
  apiCall: Promise<T>,
) {
  try {
    await apiCall;
  } catch (e: unknown) {
    expect(e).toBeInstanceOf(ResponseError);
    if (e instanceof ResponseError) {
      if (e.response.body !== null) {
        expect(e.response.status).toBe(responseCode);
        if (bodyContains != '') {
          const body = await e.response.text();
          expect(body).toContain(bodyContains);
        }
      }
    }
  }
}
