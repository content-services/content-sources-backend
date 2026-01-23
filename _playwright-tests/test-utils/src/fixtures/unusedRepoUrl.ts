import { RepositoriesApi } from '../client';
import { clientTest } from './client';

type WithUnusedRepoUrl = {
  unusedRepoUrl: () => Promise<string>;
};

export const unusedRepoUrlTest = clientTest.extend<WithUnusedRepoUrl>({
  unusedRepoUrl: async ({ client }, use, testInfo) => {
    const repoApi = new RepositoriesApi(client);

    // Each worker only considers subset of repositories to avoid races between parallel tests
    const WORKERS = testInfo.config.workers ?? 1;
    const workerIndex = testInfo.parallelIndex ?? 0;

    const TOTAL_REPOS = 100;
    const perWorker = Math.ceil(TOTAL_REPOS / WORKERS);

    const start = workerIndex * perWorker + 1;
    const end = Math.min((workerIndex + 1) * perWorker, TOTAL_REPOS);

    const getRandomRepoNumber = () => Math.floor(Math.random() * (end - start + 1)) + start;

    const usedUrls = new Set<string>();

    const getUnusedUrl = async (): Promise<string> => {
      const MAX_ATTEMPTS = end - start + 1;

      for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
        const num = getRandomRepoNumber();
        const url = `https://content-services.github.io/fixtures/yum/centirepos/repo${num
          .toString()
          .padStart(2, '0')}/`;

        // Skip if already returned in this test
        if (usedUrls.has(url)) {
          continue;
        }

        try {
          const response = await repoApi.listRepositories({
            origin: 'external',
            search: url,
          });

          if (response.meta?.count === 0) {
            usedUrls.add(url);
            return url;
          }
        } catch (error) {
          console.error(`Error checking URL ${url}:`, error);
          throw new Error('Failed to verify URL availability');
        }
      }
      throw new Error(
        `Worker ${workerIndex} could not find a free repo in its range ${start}-${end}`,
      );
    };
    await use(getUnusedUrl);
  },
});
