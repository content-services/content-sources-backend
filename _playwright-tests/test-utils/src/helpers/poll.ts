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
