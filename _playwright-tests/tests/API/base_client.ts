// Define a fixture to hold the API client
import { test as oldTest, expect, mergeTests } from '@playwright/test';
import { Configuration, ResponseContext, ResponseError } from './client';
import { setGlobalDispatcher, ProxyAgent } from 'undici';
import { cleanupTest } from './fixtures/cleanup';

type WithApiConfig = {
  client: Configuration;
};

// Default error handling doesn't print the error, so print it here
const responseReader = {
  post: async function (context: ResponseContext): Promise<void> {
    if (context.response != undefined && context.response.status > 300) {
      const bodyText = await context.response.text();
      console.log('Response errored with ' + context.response.status + ': ' + bodyText);
    }
  },
};

const clientTest = oldTest.extend<WithApiConfig>({
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

      await use(client);
    },
});

export const test = mergeTests(clientTest, cleanupTest);

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
