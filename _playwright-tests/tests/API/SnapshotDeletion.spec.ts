import { expect, test } from 'test-utils';
import {
  ApiRepositoryResponse,
  ApiTaskInfoResponse,
  GetRepositoryRequest,
  GetTaskRequest,
  ListTasksRequest,
  RepositoriesApi,
  SnapshotsApi,
  TasksApi,
  TemplatesApi,
  CreateTemplateRequest,
  ApiTemplateResponse,
  BulkDeleteSnapshotsRequest,
  DeleteSnapshotRequest,
  ApiSnapshotCollectionResponse,
  ResponseError,
} from 'test-utils/client';
import { cleanupRepositories, cleanupTemplates, poll } from 'test-utils/helpers';

test.describe('Snapshot Deletion', () => {
  test('Test snapshot deletion functionality', async ({ client, cleanup }) => {
    test.setTimeout(300_000); // 5 minutes for complex operations

    const repoName = `snapshot-deletion-test`;
    const templateName = `snapshot-deletion-template`;

    // Use repos with different content to trigger snapshots
    const initialUrl = `https://jlsherrill.fedorapeople.org/fake-repos/revision/one/`;

    // Use CodeReady repo for RHEL repo (needed for template creation)
    const codeReadyRepoName = 'Red Hat CodeReady Linux Builder for RHEL 9 ARM 64 (RPMs)';

    await cleanup.runAndAdd(() => cleanupRepositories(client, repoName));
    await cleanup.runAndAdd(() => cleanupTemplates(client, templateName));

    let repo: ApiRepositoryResponse;
    let codeReadyUuid: string;

    await test.step('Get CodeReady repo UUID for template creation', async () => {
      const codeReadyRepos = await new RepositoriesApi(client).listRepositories({
        origin: 'red_hat',
        search: codeReadyRepoName,
      });
      expect(codeReadyRepos.data?.length).toBeGreaterThan(0);
      codeReadyUuid = codeReadyRepos.data![0].uuid!;
    });

    await test.step('Create repository with snapshots enabled', async () => {
      repo = await new RepositoriesApi(client).createRepository({
        apiRepositoryRequest: {
          name: repoName,
          url: initialUrl,
          snapshot: true,
        },
      });

      expect(repo.uuid).toBeDefined();
      expect(repo.name).toBe(repoName);
      expect(repo.url).toBe(initialUrl);
      expect(repo.snapshot).toBe(true);
    });

    await test.step('Wait for initial repository to be valid', async () => {
      const getRepository = () =>
        new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
          uuid: repo.uuid?.toString(),
        });
      const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
      const validRepo = await poll(getRepository, waitWhilePending, 30);
      expect(validRepo.status).toBe('Valid');
      repo = validRepo;
    });

    await test.step('Create four snapshots by editing repository URL', async () => {
      const repoNames = ['two', 'three', 'four'];
      for (let i = 0; i < repoNames.length; i++) {
        // Use repos with different content to ensure each update triggers a new snapshot
        const updatedUrl = `https://jlsherrill.fedorapeople.org/fake-repos/revision/${repoNames[i]}/`;

        const updatedRepo = await new RepositoriesApi(client).partialUpdateRepository({
          uuid: repo.uuid!,
          apiRepositoryUpdateRequest: {
            url: updatedUrl,
          },
        });

        expect(updatedRepo.url).toBe(updatedUrl);
        repo = updatedRepo;

        // Wait for repository to be valid after each edit
        const getRepository = () =>
          new RepositoriesApi(client).getRepository(<GetRepositoryRequest>{
            uuid: repo.uuid?.toString(),
          });
        const waitWhilePending = (resp: ApiRepositoryResponse) => resp.status === 'Pending';
        const validRepo = await poll(getRepository, waitWhilePending, 30);
        expect(validRepo.status).toBe('Valid');
        repo = validRepo;

        // Verify snapshot task completed
        if (repo.lastSnapshotTaskUuid) {
          const getTask = () =>
            new TasksApi(client).getTask(<GetTaskRequest>{
              uuid: repo.lastSnapshotTaskUuid!,
            });
          const waitWhileRunning = (task: ApiTaskInfoResponse) => task.status !== 'completed';
          const completedTask = await poll(getTask, waitWhileRunning, 30);
          expect(completedTask.status).toBe('completed');
        }
      }
    });

    await test.step('Verify four snapshots exist for the repository', async () => {
      const snapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(snapshots.meta?.count).toBe(4);
      expect(snapshots.data?.length).toBe(4);
    });

    let template: ApiTemplateResponse;
    let snapshotsForRepo: ApiSnapshotCollectionResponse;

    await test.step('Create template that uses the repository', async () => {
      // Get current snapshots before template creation
      snapshotsForRepo = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });

      template = await new TemplatesApi(client).createTemplate(<CreateTemplateRequest>{
        apiTemplateRequest: {
          name: templateName,
          repositoryUuids: [codeReadyUuid, repo.uuid!],
          description: 'Test template for snapshot deletion',
          arch: 'aarch64',
          version: '9',
          useLatest: false,
          // Use a date in the past to get a specific snapshot
          date: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(), // 1 day ago
        },
      });

      expect(template.uuid).toBeDefined();
      expect(template.name).toBe(templateName);
    });

    await test.step('Verify template uses the latest snapshot', async () => {
      const templateDetails = await new TemplatesApi(client).getTemplate({
        uuid: template.uuid!,
      });

      // Template should reference one of the repository snapshots
      expect(templateDetails.snapshots?.length).toBeGreaterThan(0);
      const repoSnapshotInTemplate = templateDetails.snapshots?.find(
        (snap) => snap.repositoryUuid === repo.uuid,
      );
      expect(repoSnapshotInTemplate).toBeDefined();

      // Verify it's using the fourth (latest) snapshot
      // Snapshots are ordered by creation time, so the last one should be the latest
      const latestSnapshot = snapshotsForRepo.data![snapshotsForRepo.data!.length - 1];
      expect(repoSnapshotInTemplate?.uuid).toBe(latestSnapshot.uuid);
    });

    let initialDeleteTaskCount: number;

    await test.step('Get initial delete-snapshots task count', async () => {
      const initialTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        type: 'delete-snapshots',
        status: 'completed',
      });
      initialDeleteTaskCount = initialTasks.meta?.count || 0;
    });

    await test.step('Delete a single snapshot', async () => {
      const snapshotToDelete = snapshotsForRepo.data![0]; // Delete the first (oldest) snapshot

      await new SnapshotsApi(client).deleteSnapshot(<DeleteSnapshotRequest>{
        repoUuid: repo.uuid!,
        snapshotUuid: snapshotToDelete.uuid,
      });

      // Verify snapshot is deleted
      const updatedSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(updatedSnapshots.meta?.count).toBe(3);

      // Verify the deleted snapshot is not in the list
      const deletedSnapshotExists = updatedSnapshots.data?.some(
        (snap) => snap.uuid === snapshotToDelete.uuid,
      );
      expect(deletedSnapshotExists).toBe(false);
    });

    await test.step('Verify delete-snapshots task was created and completed', async () => {
      await poll(
        async () => {
          const tasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'delete-snapshots',
            status: 'completed',
          });
          return tasks;
        },
        (resp) => (resp.meta?.count || 0) < initialDeleteTaskCount + 1,
        30,
      );

      const completedTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        type: 'delete-snapshots',
        status: 'completed',
      });
      expect(completedTasks.meta?.count).toBe(initialDeleteTaskCount + 1);
    });

    await test.step('Bulk delete multiple snapshots', async () => {
      const currentSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });

      // Delete 2 snapshots, leaving 1 (can't delete all)
      const snapshotsToDelete = currentSnapshots.data!.slice(0, 2);
      const snapshotUuidsToDelete = snapshotsToDelete.map((snap) => snap.uuid!);

      await new SnapshotsApi(client).bulkDeleteSnapshots(<BulkDeleteSnapshotsRequest>{
        repoUuid: repo.uuid!,
        apiUUIDListRequest: {
          uuids: snapshotUuidsToDelete,
        },
      });

      // Verify snapshots are deleted
      const finalSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(finalSnapshots.meta?.count).toBe(1);

      // Verify deleted snapshots are not in the list
      for (const deletedUuid of snapshotUuidsToDelete) {
        const deletedSnapshotExists = finalSnapshots.data?.some(
          (snap) => snap.uuid === deletedUuid,
        );
        expect(deletedSnapshotExists).toBe(false);
      }
    });

    await test.step('Verify bulk delete task was created and completed', async () => {
      // Wait for second delete task to complete
      await poll(
        async () => {
          const tasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
            type: 'delete-snapshots',
            status: 'completed',
          });
          return tasks;
        },
        (resp) => (resp.meta?.count || 0) < initialDeleteTaskCount + 2,
        30,
      );

      const completedTasks = await new TasksApi(client).listTasks(<ListTasksRequest>{
        type: 'delete-snapshots',
        status: 'completed',
      });
      expect(completedTasks.meta?.count).toBe(initialDeleteTaskCount + 2);
    });

    await test.step('Verify template now uses the remaining snapshot', async () => {
      const updatedTemplate = await new TemplatesApi(client).getTemplate({
        uuid: template.uuid!,
      });

      const remainingSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      const remainingSnapshot = remainingSnapshots.data![0];

      // Template should now reference the remaining snapshot
      const repoSnapshotInTemplate = updatedTemplate.snapshots?.find(
        (snap) => snap.repositoryUuid === repo.uuid,
      );
      expect(repoSnapshotInTemplate?.uuid).toBe(remainingSnapshot.uuid);
    });

    await test.step('Test protection against deleting the last snapshot', async () => {
      const remainingSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(remainingSnapshots.data?.length).toBe(1);

      const lastSnapshot = remainingSnapshots.data![0];

      // Attempt to delete the last snapshot should fail
      let deleteError: ResponseError | undefined;
      try {
        await new SnapshotsApi(client).deleteSnapshot(<DeleteSnapshotRequest>{
          repoUuid: repo.uuid!,
          snapshotUuid: lastSnapshot.uuid!,
        });
      } catch (error) {
        deleteError = error as ResponseError;
      }

      // Should have thrown an error
      expect(deleteError).toBeDefined();
      expect(deleteError!.response.status).toBe(400); // Bad Request or similar error code
    });

    await test.step('Verify the last snapshot still exists after failed deletion', async () => {
      const finalSnapshots = await new SnapshotsApi(client).listSnapshotsForRepo({
        uuid: repo.uuid!,
      });
      expect(finalSnapshots.meta?.count).toBe(1);
    });
  });
});
