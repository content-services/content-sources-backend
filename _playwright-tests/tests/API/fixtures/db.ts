import { test as oldTest } from '@playwright/test';
import { Client, QueryResult } from 'pg';

type WithDatabase = {
  db: Database;
};

export interface Database {
  executeQuery: (query: string) => Promise<QueryResult | undefined>;
}

const database: Database = {
  executeQuery: async (query: string) => {
    const connectionString = process.env['DB_CONNECTION_STRING'];
    if (!connectionString || connectionString.length == 0) {
      return;
    }

    const client = new Client({ connectionString: connectionString });
    try {
      await client.connect();
      const result = await client.query(query);
      return result;
    } catch (error) {
      console.error('Error in connection/executing query:', error);
    } finally {
      await client.end().catch((error) => {
        console.error('Error ending client connection:', error);
      });
    }
  },
};

export const databaseTest = oldTest.extend<WithDatabase>({
  // eslint-disable-next-line no-empty-pattern
  db: async ({}, use) => {
    await use(database);
  },
});
