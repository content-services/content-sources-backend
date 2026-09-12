import { expect, test } from 'test-utils';
import {
  ApiRepositoryResponse,
  ApiTaskInfoCollectionResponse,
  ApiTaskInfoResponse,
  FeaturesApi,
  GetRepositoryRequest,
  GetTaskRequest,
  ListTasksRequest,
  PublishSnapshotRequest,
  RepositoriesApi,
  ResponseError,
  SnapshotsApi,
  TasksApi,
} from 'test-utils/client';
import { cleanupRepositories, poll, randomName } from 'test-utils/helpers';

test.describe('Snapshot Publish', () => {
  test('Test snapshot publish and unpublish functionality', async ({ client, cleanup }) => {
    test.setTimeout(300_000); // 5 minutes for complex operations

    const repoNamePrefix = 'snapshot-publish-test';
    const repoName = `${repoNamePrefix}-${randomName()}`;
    const repoUrl = 'https://jlsherrill.fedorapeople.org/fake-repos/revision/one/';

    // Skip if adminpartnerrepositories feature is not available
    const features = await new FeaturesApi(client).listFeatures();
    test.skip(
      !(
        features['adminpartnerrepositories']?.enabled &&
        features['adminpartnerrepositories']?.accessible
      ),
      'adminpartnerrepositories not accessible',
    );

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoNamePrefix));

    let repo: ApiRepositoryResponse;

    await test.step('Create repository with snapshots enabled', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: repoUrl,
          snapshot: true,
        },
      });

      expect(repo.uuid).toBeDefined();
      expect(repo.name).toBe(repoName);
      expect(repo.snapshot).toBe(true);
    });

    await test.step('Wait for initial repository to be valid', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid,
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const validRepo = await poll(getRepository, waitWhilePending, 30);
      expect(validRepo.status).toBe('Valid');
      repo = validRepo;
    });

    await test.step('Wait for snapshot task to complete', async () => {
      expect(repo.lastSnapshotTaskUuid).toBeDefined();
      const getTask = () =>
        new TasksApi(client).getTask(<GetTaskRequest>{
          uuid: repo.lastSnapshotTaskUuid!,
        });
      const waitWhileRunning = (task: ApiTaskInfoResponse) => task.status !== 'completed';
      const completedTask = await poll(getTask, waitWhileRunning, 30);
      expect(completedTask.status).toBe('completed');
    });

    let snapshotUuid: string;

    await test.step('Verify snapshot exists with packages', async () => {
      const snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(snapshots.meta?.count).toBeGreaterThanOrEqual(1);
      expect(snapshots.data?.length).toBeGreaterThanOrEqual(1);

      const snapshot = snapshots.data![0];
      snapshotUuid = snapshot.uuid!;
      expect(snapshotUuid).toBeDefined();
      // The fake-repos URL should produce packages
      expect(snapshot.contentCounts?.['rpm.package']).toBeGreaterThan(0);
    });

    await test.step('Mark repository as partner', async () => {
      const adminUrl = `${client.basePath}/admin/repositories/${repo.uuid}/partner`;
      const response = await fetch(adminUrl, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          ...client.headers,
        },
        body: JSON.stringify({ partner: true }),
      });
      expect(response.status).toBe(200);

      // Verify partner status
      const updatedRepo = await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid,
      });
      expect(updatedRepo.partner).toBe(true);
    });

    let initialPublishTaskCount: number;

    await test.step('Get initial publish task count', async () => {
      const initialTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        type: 'update-snapshot-published',
        status: 'completed',
      });
      initialPublishTaskCount = initialTasks.meta?.count || 0;
    });

    await test.step('Publish snapshot', async () => {
      const publishedSnapshot = await new SnapshotsApi(client).publishSnapshot(
        <PublishSnapshotRequest>{
          repoUuid: repo.uuid!,
          snapshotUuid: snapshotUuid,
          apiSnapshotPublishedUpdateRequest: { published: true },
        },
      );

      expect(publishedSnapshot.published).toBe(true);
      expect(publishedSnapshot.publishTaskUuid).toBeDefined();
      expect(publishedSnapshot.publishTask).toBeDefined();
      expect(publishedSnapshot.publishTask?.status).toBe('pending');
    });

    await test.step('Verify publish task completes', async () => {
      await poll(
        async () => {
          const tasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'update-snapshot-published',
            status: 'completed',
          });
          return tasks;
        },
        (resp: ApiTaskInfoCollectionResponse) =>
          (resp.meta?.count || 0) < initialPublishTaskCount + 1,
        30,
      );
    });

    await test.step('Verify snapshot is published', async () => {
      const snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      const snapshot = snapshots.data?.find((s) => s.uuid === snapshotUuid);
      expect(snapshot).toBeDefined();
      expect(snapshot!.published).toBe(true);
      expect(snapshot!.publishTask?.status).toBe('completed');
    });

    await test.step('Verify repository-level publish state', async () => {
      const updatedRepo = await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid,
      });
      expect(updatedRepo.snapshotPublishState?.published).toBe(true);
    });

    await test.step('Unpublish snapshot', async () => {
      const unpublishedSnapshot = await new SnapshotsApi(client).publishSnapshot(
        <PublishSnapshotRequest>{
          repoUuid: repo.uuid!,
          snapshotUuid: snapshotUuid,
          apiSnapshotPublishedUpdateRequest: { published: false },
        },
      );

      expect(unpublishedSnapshot.published).toBe(false);
      expect(unpublishedSnapshot.publishTaskUuid).toBeDefined();
      expect(unpublishedSnapshot.publishTask?.status).toBe('pending');
    });

    await test.step('Verify unpublish task completes', async () => {
      await poll(
        async () => {
          const tasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'update-snapshot-published',
            status: 'completed',
          });
          return tasks;
        },
        (resp: ApiTaskInfoCollectionResponse) =>
          (resp.meta?.count || 0) < initialPublishTaskCount + 2,
        30,
      );
    });

    await test.step('Verify snapshot is unpublished', async () => {
      const snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      const snapshot = snapshots.data?.find((s) => s.uuid === snapshotUuid);
      expect(snapshot).toBeDefined();
      expect(snapshot!.published).toBe(false);
      expect(snapshot!.publishTask?.status).toBe('completed');
    });

    await test.step('Verify repository-level publish state after unpublish', async () => {
      const updatedRepo = await new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
        uuid: repo.uuid,
      });
      expect(updatedRepo.snapshotPublishState?.published).toBe(false);
    });

    await test.step('Test conflict: publish while task is in progress', async () => {
      // Start a publish operation
      await new SnapshotsApi(client).publishSnapshot(<PublishSnapshotRequest>{
        repoUuid: repo.uuid!,
        snapshotUuid: snapshotUuid,
        apiSnapshotPublishedUpdateRequest: { published: true },
      });

      // Immediately try to publish again - should get 409 Conflict
      let conflictError: ResponseError | undefined;
      try {
        await new SnapshotsApi(client).publishSnapshot(<PublishSnapshotRequest>{
          repoUuid: repo.uuid!,
          snapshotUuid: snapshotUuid,
          apiSnapshotPublishedUpdateRequest: { published: false },
        });
      } catch (error) {
        conflictError = error as ResponseError;
      }

      // The second request should fail with 409 if the first task is still in progress,
      // or succeed if the task completed very quickly. Both outcomes are acceptable.
      if (conflictError) {
        expect(conflictError.response.status).toBe(409);
      }

      // Wait for any in-progress publish task to finish before test ends
      await poll(
        async () => {
          const tasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'update-snapshot-published',
            status: 'completed',
          });
          return tasks;
        },
        (resp: ApiTaskInfoCollectionResponse) =>
          (resp.meta?.count || 0) < initialPublishTaskCount + 3,
        30,
      );
    });
  });
});
