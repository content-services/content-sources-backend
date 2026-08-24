
Jobs are simple commands that can be run in our stage and production environments.

To execute them:

1. Define the job in `./deployments/build/deployment.template.yaml` under `spec.jobs`.
2. Run `make deployment-generate` so `./deployments/deployment.yaml` stays in sync (do not edit it by hand).
3. Add a `ClowdJobInvocation` in `./deployments/jobs-stage.yaml` and `./deployments/jobs-prod.yaml`.

If a job needs to re-run, bump the `metadata.name` date suffix on the `ClowdJobInvocation`. The ClowdApp with the job name must be deployed before the invocation, or Clowder cannot find the job.

`./cmd/jobs/main.go` is utilized to run them.

## send-template-update-events

One-shot backfill. Command: `/jobs send-template-update-events`.

Loads every non-deleted template and publishes a `com.redhat.console.repositories.template-updated` CloudEvent to `platform.content-sources.template`. Current create, update, and delete events already emit the same event from the DAO; this job is only needed to backfill existing templates.

### Verification after invocation

**Content Sources job logs**

- Pod succeeds.
- `Found templates to process` `template_count` matches non-deleted templates.
- `Finished sending template-updated events` with `events_sent` equal to `total_templates` (or account for per-template fetch errors).

**Patch**

Patch listener consumes `platform.content-sources.template`. For each `template-updated` event it upserts the `template` row, calls `GET /templates/{uuid}/advisories/ids`, and diffs the result into `template_advisory`. See [patchman-engine template flow](https://github.com/RedHatInsights/patchman-engine/blob/master/docs/md/architecture.md).

Spot-check Patch DB: a sampled template UUID exists in `template`, and `template_advisory` rows match that template's advisory IDs from Content Sources.

If `template_advisory_eval` is enabled on Patch, systems assigned to those templates may be re-evaluated; that is Patch behavior, not this job.
