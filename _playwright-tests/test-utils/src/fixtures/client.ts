// Define a fixture to hold the API client
import { test as oldTest, expect, APIRequestContext, APIResponse } from '@playwright/test';
import { Configuration, ResponseError, FetchAPI } from '../client';
import { fileNameToEnvVar, getFileNameFromAuthPath } from 'test-utils/helpers';

type WithApiConfig = {
  client: Configuration;
};

function isString(value: unknown): value is string {
  return typeof value === 'string';
}

function constructHeaders(headers?: HeadersInit, storageState?: string, extraHTTPHeaders?: {[key: string]: string}) {
  const out: Record<string, string> = {};

  if (storageState) {
    const tokenName = fileNameToEnvVar(getFileNameFromAuthPath(storageState));
    const token = process.env[tokenName];
    if (!token) {
      throw new Error(
        `Token environment variable "${tokenName}" is not set. Check authentication setup.`,
      );
    }
    out['authorization'] = token;
  } else if (extraHTTPHeaders) {
    new Headers(extraHTTPHeaders).forEach((value, key) => { out[key] = value; });
  }

  if (headers) {
    new Headers(headers).forEach((value, key) => { out[key] = value; });
  };

  return Object.keys(out).length ? out : undefined;
}

async function toFetchResponseBody(
  response: APIResponse,
): Promise<BodyInit | undefined> {
  const nullBodyResponses = new Set([101, 103, 204, 205, 304]);
  if (nullBodyResponses.has(response.status())) return undefined;

  return response.body().then((b) => b.toString());
}

export const clientTest = oldTest.extend<WithApiConfig>({
  client:
    async ({ extraHTTPHeaders, request, storageState }, use, r) => {
      const pwFetch = (api: APIRequestContext): FetchAPI => async (url, init) => {
        const storage = storageState ?? r.project.use.storageState;
        const extraHeaders = extraHTTPHeaders ?? r.project.use.extraHTTPHeaders;
        const response = await api.fetch(String(url), {
          failOnStatusCode: false,
          ignoreHTTPSErrors: true,
          method: init?.method,
          headers: constructHeaders(init?.headers, isString(storage) ? storage : undefined, extraHeaders),
          data: init?.body,
        });

        try {
          return new Response(await toFetchResponseBody(response), {
            status: response.status(),
            statusText: response.statusText(),
            headers: response.headers(),
          });
        } finally {
          await response.dispose();
        }
      };

      const client = new Configuration({
        fetchApi: pwFetch(request),
        basePath: r.project.use.baseURL + '/api/content-sources/v1',
        middleware: [],
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
