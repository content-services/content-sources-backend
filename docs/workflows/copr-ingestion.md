# Copr Build Ingestion

## Overview

Copr is a build system that publishes its build results into Pulp. Whenever a build finishes, Copr sends a build end event over Kafka.
Content Sources listens for these events and turns each successful build into a snapshot, so RHEL customers can use Copr content as a partner repository.

In short: **Copr builds and publishes the content, Content Sources snapshots and serves it.**

## The Copr Ecosystem

### What Copr Is

[Copr](https://docs.copr.fedorainfracloud.org/user_documentation.html) is Fedora's community build system. Users submit sources, and Copr builds RPMs and serves them as yum repositories.

The terms used in the rest of this doc:

- **Project** — where builds happen, named `<owner>/<project>`. The owner is a user or a group; group names start with `@`, e.g. `@theforeman/foreman-nightly-staging`.
- **Chroot** — a build target, named `<distro>-<version>-<arch>`, e.g., `rhel-9-x86_64` or `fedora-43-aarch64`. The distro can contain dashes (`centos-stream-9-x86_64`).
- **Build** — one run of a project's sources. It has one build ID and runs once in every chroot the project enables. Each chroot succeeds or fails on its own.
- **Repository** — Copr keeps one repository per project per chroot. Each build adds to it, so the latest version also contains everything earlier builds produced.

### Build End Events

Copr sends a build end event for every chroot a build finishes in, not once per build.
So a build for three chroots produces three events. They share the build ID and differ only by chroot (and possibly status).

Copr sends other events too, such as build start, but Content Sources only handles build end events for now and skips other types.

### Copr on Pulp and packages.redhat.com

- Copr is [migrating its build results to Pulp](https://fedora-copr.github.io/posts/migrating-copr-results-to-pulp), project by project.
- For Red Hat, Copr is moving onto `packages.redhat.com`. There it works in the Copr Pulp domain (`cs-partner`), which is the same domain Content Sources reads from.
- The events come through the IT Managed Kafka instance for now, and through the Hybrid Cloud Console (HCC) Kafka once Copr is onboarded there. The consumer is only set up for HCC Kafka so far, see [Kafka Setup](#kafka-setup).

### Copr as a Partner

The [partner repositories program](https://redhat.gitlab.io/partner-repositories/docs/) delivers partner and layered-product software to RHEL customers through Red Hat infrastructure. Copr is one of the partners.

In Content Sources, partner repositories are upload repositories marked with `partner = true`. Their published snapshots are visible to every org. See [Partner Repository Publishing](https://github.com/content-services/content-sources-frontend/blob/main/docs/domain/partner-repos.md) for how publishing works.

## Our Role in the Pipeline

### Who Owns What in Pulp

Repository → Repository Version → Publication (generated yum metadata) → Distribution (URL path serving a publication).

```
Copr repository
  └─ repository versions
       └─ publications
            ↑ referenced by
       Content Sources distributions
```

- **Copr** owns the repository, adds a repository version for each build, and publishes each version.
- **Content Sources** only creates a distribution for each snapshot, pointing at Copr's publication. It never syncs Copr content or creates publications for it.

### Ingestion Flow

1. **Copr** finishes a build in a chroot, adds a repository version, publishes it, and sends a build end event with the repository and publication hrefs.
2. **copr-ingest consumer** (runs in the `service` deployment, next to the API):
   - Decodes and validates the event.
   - Skips other event types, failed builds, unsupported chroots, and invalid messages.
   - Enqueues a `create-partner-snapshot` task for the Copr org (`-5`). The task's request ID is the event ID, so one search in the logs finds both.
3. **create-partner-snapshot task** (runs in the task workers):
   - Finds the repository for the Copr repository href, or creates it.
   - Stops if a snapshot already exists for that repository version, so a duplicate event doesn't create a second snapshot.
   - Creates the snapshot: reuses Copr's publication and creates a distribution at `<repoUUID>/<snapshotIdent>`.
   - Stores the Copr build ID on the snapshot.

### Example

Say a `@theforeman/foreman-nightly-staging` build runs in `rhel-9-x86_64`, `rhel-9-aarch64`, and `fedora-43-x86_64`, and Copr sends three build end events with the same build ID:

- `rhel-9-x86_64` → snapshot in the repository `@theforeman/foreman-nightly-staging for RHEL 9 x86_64` (created on the first build).
- `rhel-9-aarch64` → snapshot in the repository `@theforeman/foreman-nightly-staging for RHEL 9 aarch64`.
- `fedora-43-x86_64` → skipped, Fedora chroots aren't supported.

## Consumer Rules

### Supported Chroots

A chroot is supported when its distro is listed in `CoprChrootDistros` and Content Sources supports its version and architecture.

- Only `rhel` is listed for now, so `rhel-9-x86_64` is supported, while `epel-9-x86_64` and `fedora-43-aarch64` are skipped.
- To support another distro, add it to `CoprChrootDistros`, e.g., `"epel": "EPEL"`.

### Repository Names

- Format: `<owner>/<project> for <Distro> <version> <arch>`, e.g., `@theforeman/foreman-nightly-staging for RHEL 9 aarch64`.
- Names are unique per org and at most 255 characters. A longer name makes the message invalid, so it's skipped.
- The owner is part of the name, because project names are only unique per owner, so two owners can have a project of the same name.

## Key DB Fields

Repositories:

- `repository_configurations.org_id` — `-5` for every Copr repository. The Copr org owns them, not the customer orgs that use them.
- `repository_configurations.origin` — `upload`, because partner repositories are upload repositories. `partner` isn't set for Copr repositories yet.
- `repository_configurations.copr_project_id` — the Copr project the repository belongs to, `<owner>/<project>` (e.g., `@theforeman/foreman-nightly-staging`). A project builds for several chroots, so several repositories share the same project ID.
- `repository_configurations.copr_repository_href` — the Pulp repository Copr builds into for one project and chroot. It stays the same across builds, so it tells which of our repositories a build end event belongs to. Unique, and only set on Copr repositories.

Snapshots:

- `snapshots.copr_build_id` — the Copr build that produced the snapshot's content. One build runs in several chroots, so the same build ID appears on one snapshot in each chroot's repository.

## Configuration

- `copr_ingest.enabled` — turns the consumer on (default `false`). It runs as the `copr-ingest` option of `content-sources`.
- `copr_ingest.topic` — the Kafka topic with the build end events (default `copr.build.events`).
- `copr_ingest.consumer_group_id` — the Kafka consumer group (default `content-sources-copr`).
- `clients.pulp.copr_domain` — the Copr Pulp domain (default `cs-partner`).

### Kafka Setup

The consumer is configured with HCC-like settings, so this setup assumes Copr is already onboarded to HCC Kafka:

- It reads from our own cluster, with the shared `kafka` brokers and auth settings (from Clowder in the deployments), and Clowder provisions the `COPR_INGEST_TOPIC` topic there.
- It expects structured-mode CloudEvents, as other HCC services send.

Copr currently publishes to the IT-managed Kafka instance, while the consumer connects to the HCC Kafka cluster, so it receives no events. The integration details, such as separate broker and authentication settings for copr-ingest, still need to be agreed with the Copr team.

## Metrics

The consumer's metrics are served on the `instrumentation` metrics endpoint (`metrics.path` on `metrics.port`).

- `content_sources_copr_build_end_events_total{result}`: build end events by result:
  - `enqueued`: a snapshot task was enqueued.
  - `invalid`: skipped, because of the message content (e.g., an invalid CloudEvent, missing fields, a malformed chroot that isn't `<distro>-<version>-<arch>`, or a repository name longer than 255 characters).
  - `failed`: skipped, because handling a valid message failed (e.g., the task enqueue failed after retries).
- `content_sources_copr_event_delay_seconds`: time from when Copr sent the event (the CloudEvent `time`) to when it was handled, shared by all event types on the topic. Events without a `time` aren't observed.

Expected skips aren't counted in `content_sources_copr_build_end_events_total`: other event types, failed builds, and unsupported chroots are only logged at debug level.

A message that is consumed again because the session ended while handling it isn't counted until it's handled.
