// Define a fixture to hold the API client
import { test as oldTest, expect } from '@playwright/test';
import { Configuration, ResponseContext, ResponseError } from '../client';
import { setGlobalDispatcher, ProxyAgent, Agent } from 'undici';

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

export const clientTest = oldTest.extend<WithApiConfig>({
  client:
    // eslint-disable-next-line no-empty-pattern
    async ({ extraHTTPHeaders }, use, r) => {
      if (r.project?.use?.proxy?.server) {
        const dispatcher = new ProxyAgent({ uri: new URL(r.project.use.proxy.server).toString() });
        setGlobalDispatcher(dispatcher);
      } else if (r.project.use.baseURL?.match('^https:\/\/\\w+\.foo\.redhat\.com:\\d+$')?.length) {
        const dispatcher = new Agent({ connect: { rejectUnauthorized: false } });
        setGlobalDispatcher(dispatcher);
      }

      const client = new Configuration({
        basePath: r.project.use.baseURL + '/api/content-sources/v1',
        headers: extraHTTPHeaders ?? r.project.use.extraHTTPHeaders,
        middleware: [responseReader],
      });

      await use(client);
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
