// while condition is true, calls fn, waits interval (ms) between calls.
// condition's parameter should be the result of the function call.
export const poll = async (
  fn: <T>() => Promise<T>,
  condition: <T>(result: T) => boolean,
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
