import { expect, test } from 'test-utils';
import { randomUUID } from 'crypto';
import {
  ApiRepositoryCollectionResponse,
  ApiTaskInfoCollectionResponse,
  ListRepositoriesRequest,
  ListTasksRequest,
  RepositoriesApi,
  TasksApi,
  ApiRepositoryResponse,
  GetRepositoryRequest,
  RpmsApi,
  SnapshotsApi,
} from 'test-utils/client';
import { cleanupRepositories, poll, randomName, randomUrl } from 'test-utils/helpers';
import util from 'node:util';
import child_process from 'node:child_process';
const exec = util.promisify(child_process.exec);

test.describe('Snapshots', () => {
  test('Verify snapshot cleanup', { tag: '@local-only' }, async ({ client, cleanup, db }) => {
    test.setTimeout(60_000);
    let repoUuid01: string;
    const repoUrl01 = randomUrl();
    const repoName01 = `snapshot-cleanup-${randomName()}`;
    let repoUuid02: string;
    const repoUrl02 = randomUrl();
    const repoName02 = `snapshot-cleanup-${randomName()}`;
    const toBeDeletedSnapshots: string[] = [];

    await cleanup.runAndAdd(() =>
      cleanupRepositories(client, repoUrl01, repoName01, repoUrl02, repoName02),
    );

    await test.step('Create testing repos', async () => {
      const repo01 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName01,
          url: repoUrl01,
          snapshot: true,
        },
      });

      expect(repo01.uuid).toBeDefined();
      repoUuid01 = repo01.uuid!;
      expect(repo01.name).toBe(repoName01);
      expect(repo01.url).toBe(repoUrl01);

      const repo02 = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName02,
          url: repoUrl02,
          snapshot: true,
        },
      });

      expect(repo02.uuid).toBeDefined();
      repoUuid02 = repo02.uuid!;
      expect(repo02.name).toBe(repoName02);
      expect(repo02.url).toBe(repoUrl02);

      const waitWhilePending = (resp: ApiRepositoryCollectionResponse) =>
        resp.data?.filter((r) => r.status === 'Valid').length !== 2;
      const getRepository = () =>
        new RepositoriesApi(client).listRepositories(<ListRepositoriesRequest>{
          limit: 2,
          search: 'snapshot-cleanup-',
        });
      await poll(getRepository, waitWhilePending, 100);
    });

    await test.step('Add old snapshots', async () => {
      const now = new Date();
      const soonToBeOutdated = new Date();
      soonToBeOutdated.setDate(now.getDate() - 360);
      const outdatedDate = new Date();
      outdatedDate.setDate(now.getDate() - 370);

      // Repository 01 snaps: 1 soft-deleted outdated, 1 soon to be outdated
      toBeDeletedSnapshots.push(randomUUID());
      await db.executeQuery(
        `INSERT INTO snapshots (
          uuid, created_at, updated_at, deleted_at, repository_configuration_uuid, version_href,
          publication_href, distribution_href, distribution_path
        ) VALUES (
          '${toBeDeletedSnapshots[0]}', '${outdatedDate.toISOString()}',
          '${outdatedDate.toISOString()}', '${soonToBeOutdated.toISOString()}', '${repoUuid01}',
          '', '', '/${randomName()}/${randomName()}', '/${randomName()}/${randomName()}'
        );`,
      );

      await db.executeQuery(
        `INSERT INTO snapshots (
          uuid, created_at, updated_at, repository_configuration_uuid, version_href,
          publication_href, distribution_href, distribution_path
        ) VALUES (
          '${randomUUID()}', '${soonToBeOutdated.toISOString()}',
          '${soonToBeOutdated.toISOString()}', '${repoUuid01}', '', '',
          '/${randomName()}/${randomName()}', '/${randomName()}/${randomName()}'
        );`,
      );

      // Repository 02 snaps: 1 soft-deleted outdated, 2 outdated
      toBeDeletedSnapshots.push(randomUUID());
      await db.executeQuery(
        `INSERT INTO snapshots (
          uuid, created_at, updated_at, deleted_at, repository_configuration_uuid, version_href,
          publication_href, distribution_href, distribution_path
        ) VALUES (
          '${toBeDeletedSnapshots[1]}', '${outdatedDate.toISOString()}',
          '${outdatedDate.toISOString()}', '${outdatedDate.toISOString()}', '${repoUuid02}',
          '', '', '/${randomName()}/${randomName()}', '/${randomName()}/${randomName()}'
        );`,
      );

      toBeDeletedSnapshots.push(randomUUID());
      await db.executeQuery(
        `INSERT INTO snapshots (
          uuid, created_at, updated_at, repository_configuration_uuid, version_href,
          publication_href, distribution_href, distribution_path
        ) VALUES (
          '${toBeDeletedSnapshots[2]}', '${outdatedDate.toISOString()}',
          '${outdatedDate.toISOString()}', '${repoUuid02}', '', '',
          '/${randomName()}/${randomName()}', '/${randomName()}/${randomName()}'
        );`,
      );

      toBeDeletedSnapshots.push(randomUUID());
      await db.executeQuery(
        `INSERT INTO snapshots (
          uuid, created_at, updated_at, repository_configuration_uuid, version_href,
          publication_href, distribution_href, distribution_path
        ) VALUES (
          '${toBeDeletedSnapshots[3]}', '${outdatedDate.toISOString()}',
          '${outdatedDate.toISOString()}', '${repoUuid02}', '', '',
          '/${randomName()}/${randomName()}', '/${randomName()}/${randomName()}'
        );`,
      );
    });

    await test.step(`Trigger cleanup and verify snapshots are deleted`, async () => {
      const mainDir = __filename.split('_playwright-tests')[0];
      const extReposPath = `${mainDir}cmd/external-repos/main.go`;
      await exec(
        `(cd ${mainDir} && OPTIONS_SNAPSHOT_RETAIN_DAYS_LIMIT=365 go run ${extReposPath} cleanup --type snapshot)`,
      );

      const waitForTasks = (resp: ApiTaskInfoCollectionResponse) =>
        resp.data?.filter((t) => t.status == 'completed').length !== 2;
      const getTask = () =>
        new TasksApi(client).listTasks(<ListTasksRequest>{
          type: 'delete-snapshots',
          limit: 2,
        });
      await poll(getTask, waitForTasks, 100);

      const result = await db.executeQuery(
        `SELECT * FROM snapshots
        WHERE repository_configuration_uuid IN ('${repoUuid01}', '${repoUuid02}');`,
      );
      expect(result?.rows.length).toBe(3);
      for (const r of result!.rows) {
        expect(toBeDeletedSnapshots.indexOf(r.uuid)).toBe(-1);
      }
    });
  });

  test('test_list_snapshot_errata', async ({ client, cleanup }) => {
    const repoName = `snapshot-errata-${randomName()}`;
    const repoUrl = 'https://stephenw.fedorapeople.org/fakerepos/multiple_errata/';

    await cleanup.runAndAdd(async () => await cleanupRepositories(client, repoName, repoUrl));

    let repo: ApiRepositoryResponse;
    await test.step('Create repo with 6 errata', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: true,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(repoUrl);
    });

    await test.step('Wait for snapshotting to complete', async () => {
      const getRepository = async () =>
        await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
    });

    let snapshotUuid: string;
    await test.step('Get the snapshot UUID', async () => {
      const snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(snapshots.data?.length).toBeGreaterThan(0);
      snapshotUuid = snapshots.data![0].uuid!;
    });

    await test.step('List snapshot errata without any filtering', async () => {
      const unfiltered_errata = await new RpmsApi(client).listSnapshotErrata({
        uuid: snapshotUuid,
      });
      // Assert errata count from response matches that of the repo
      expect(unfiltered_errata.data?.length).toBe(6);
    });

    await test.step('List snapshot errata with type and severity filters', async () => {
      const filtered_errata_by_type = await new RpmsApi(client).listSnapshotErrata({
        uuid: snapshotUuid,
        type: `security`,
      });
      const filtered_errata_by_severity = await new RpmsApi(client).listSnapshotErrata({
        uuid: snapshotUuid,
        severity: `Important`,
      });
      const filtered_errata_by_type_and_severity = await new RpmsApi(client).listSnapshotErrata({
        uuid: snapshotUuid,
        type: `security`,
        severity: `Critical`,
      });
      // Assert values match expected errata count with those filters
      expect(filtered_errata_by_type.data?.length).toBe(4);
      expect(filtered_errata_by_severity.data?.length).toBe(1);
      expect(filtered_errata_by_type_and_severity.data?.length).toBe(1);
    });
  });
});
