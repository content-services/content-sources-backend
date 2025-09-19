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
  GetTaskRequest,
  ApiTaskInfoResponse,
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

  test('Test listing snapshot errata with different filters', async ({ client, cleanup }) => {
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

  test('Enable snapshots on existing repository and verify snapshot tasks', async ({
    client,
    cleanup,
  }) => {
    const repoName = `snapshot-toggle-${randomName()}`;
    const initialUrl = randomUrl();
    const updatedUrl = randomUrl();

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName));

    let repo: ApiRepositoryResponse;
    let currentTime: Date;

    await test.step('Create repository with snapshots disabled', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: initialUrl,
          snapshot: false,
        },
      });
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(initialUrl);
      expect(repo.snapshot).toBe(false);
    });

    await test.step('Wait for initial repository introspection to complete', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
      expect(resp.lastSnapshotTaskUuid).toBeUndefined();
      repo = resp;
    });

    await test.step('Note current time for task verification', async () => {
      currentTime = new Date();
      currentTime.setMilliseconds(0);
    });

    await test.step('Enable snapshots and update URL to trigger snapshot', async () => {
      const updatedRepo = await new RepositoriesApi(client).partialUpdateRepository({
        uuid: repo.uuid!,
        apiRepositoryUpdateRequest: {
          snapshot: true,
          url: updatedUrl,
        },
      });

      expect(updatedRepo.uuid).toBe(repo.uuid);
      expect(updatedRepo.snapshot).toBe(true);
      expect(updatedRepo.url).toBe(updatedUrl);
      repo = updatedRepo;
    });

    await test.step('Wait for repository to be valid with snapshot enabled', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const resp = await poll(getRepository, waitWhilePending, 10);
      expect(resp.status).toBe('Valid');
      expect(resp.lastSnapshotTaskUuid).toBeDefined();
      repo = resp;
    });

    let snapshotTaskUuid: string;
    await test.step('Get snapshot task UUID and verify task completion', async () => {
      snapshotTaskUuid = repo.lastSnapshotTaskUuid!;

      const getTask = () =>
        new TasksApi(client).getTask(<GetTaskRequest>{
          uuid: snapshotTaskUuid,
        });
      const waitWhileRunning = (task: ApiTaskInfoResponse) => task.status !== 'completed';
      const completedTask = await poll(getTask, waitWhileRunning, 10);

      expect(completedTask.status).toBe('completed');
    });

    await test.step('Verify task fields', async () => {
      const task = await new TasksApi(client).getTask(<GetTaskRequest>{
        uuid: snapshotTaskUuid,
      });

      expect(task.error).toBe('');
      expect(task.type).toBe('snapshot');
      expect(task.objectName).toBe(repo.name);
      expect(task.objectUuid).toBe(repo.uuid);
      expect(task.status).toBe('completed');

      const taskCreatedAt = new Date(task.createdAt!);
      const taskEndedAt = new Date(task.endedAt!);
      expect(taskCreatedAt.getTime()).toBeGreaterThanOrEqual(currentTime.getTime());
      expect(taskEndedAt.getTime()).toBeGreaterThan(taskCreatedAt.getTime());
    });

    await test.step('Verify task API filtering', async () => {
      const allTasks = await new TasksApi(client).listTasks();
      const taskUuids = allTasks.data?.map((t) => t.uuid) || [];
      expect(taskUuids).toContain(snapshotTaskUuid);

      const snapshotTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        type: 'snapshot',
      });
      const snapshotTaskUuids = snapshotTasks.data?.map((t) => t.uuid) || [];
      expect(snapshotTaskUuids).toContain(snapshotTaskUuid);

      snapshotTasks.data?.forEach((task) => {
        expect(task.type).toBe('snapshot');
      });

      const repoTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        repositoryUuid: repo.uuid,
      });
      const repoTaskUuids = repoTasks.data?.map((t) => t.uuid) || [];
      expect(repoTaskUuids).toContain(snapshotTaskUuid);
      // Verify all returned tasks belong to our repository
      repoTasks.data?.forEach((task) => {
        expect(task.objectUuid).toBe(repo.uuid);
      });

      const completedTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        status: 'completed',
      });
      const completedTaskUuids = completedTasks.data?.map((t) => t.uuid) || [];
      expect(completedTaskUuids).toContain(snapshotTaskUuid);
      completedTasks.data?.forEach((task) => {
        expect(task.status).toBe('completed');
      });

      expect(repoTasks.data?.some((task) => task.objectName === repo.name)).toBe(true);
    });

    await test.step('Verify snapshot creation and count', async () => {
      // Wait for at least 1 snapshot to be created
      let snapshots;
      await poll(
        async () => {
          snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
            uuid: repo.uuid!,
          });
          return snapshots;
        },
        (resp) => (resp.data?.length || 0) < 1,
        10,
      );

      // Verify exactly 1 snapshot exists (as per IQE test)
      expect(snapshots!.meta?.count).toBe(1);
      expect(snapshots!.data?.length).toBe(1);

      // Store snapshot UUID for potential future verification
      const snapshotUuid = snapshots!.data![0].uuid!;
      expect(snapshotUuid).toBeTruthy();

      // Verify the snapshot UUID is available in repository response
      expect(repo.lastSnapshotUuid).toBeDefined();
    });
  });
});
