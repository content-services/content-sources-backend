import {
  RepositoriesApi,
  GetRepositoryRequest,
  ApiRepositoryResponse,
  Configuration,
} from '../client';

// while condition is true, calls fn, waits interval (ms) between calls.
// condition's parameter should be the result of the function call.
/* eslint-disable @typescript-eslint/no-explicit-any */
export const poll = async (
  fn: () => Promise<any>,
  condition: (result: any) => boolean,
  interval: number,
) => {
  let result = await fn();
  while (condition(result)) {
    result = await fn();
    await sleep(interval);
  }
  return result;
};

// sleep in ms, must await e.g. await sleep(num)
export const sleep = (ms: number) => new Promise((res) => setTimeout(res, ms));

/**
 * Waits for a repository introspection to complete by polling the repository status.
 * @param client - Configuration object for API client
 * @param repoUuid - UUID of the repository to check
 * @param interval - Polling interval in seconds (default: 10)
 */
export const waitWhileRepositoryIsPending = async (
  client: Configuration,
  repoUuid: string,
  interval: number = 10,
): Promise<ApiRepositoryResponse> => {
  const getRepository = () =>
    new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
      uuid: repoUuid,
    });
  const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
  const result = await poll(getRepository, waitWhilePending, interval);
  return result;
};
