import { sleep } from './poll';

export const retry = async (callback: () => Promise<unknown>, tries = 3, delay?: number) => {
  let rc = tries;
  while (rc >= 0) {
    if (delay) {
      await sleep(delay);
    }

    rc -= 1;
    if (rc === 0) {
      return await callback();
    } else {
      try {
        await callback();
      } catch {
        continue;
      }
      break;
    }
  }
};
