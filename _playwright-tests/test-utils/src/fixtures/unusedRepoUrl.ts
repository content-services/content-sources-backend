import { RepositoriesApi } from '../client';
import { generateNewUrl } from '../helpers/generateNewUrl';
import { clientTest } from './client';

type WithUnusedRepoUrl = {
  unusedRepoUrl: () => Promise<string>;
};

export const unusedRepoUrlTest = clientTest.extend<WithUnusedRepoUrl>({
  unusedRepoUrl: async ({ client }, use) => {
    const repoApi = new RepositoriesApi(client);

    const getUnusedUrl = async (): Promise<string> => {
      while (true) {
        const url = generateNewUrl();
        try {
          const response = await repoApi.listRepositories({
            origin: 'external',
            search: url,
          });
          if (response.meta?.count === 0) {
            return url;
          }
        } catch (error) {
          console.error(`Error checking URL ${url}:`, error);
          throw new Error('Failed to verify URL availability');
        }
      }
    };

    await use(getUnusedUrl);
  },
});
