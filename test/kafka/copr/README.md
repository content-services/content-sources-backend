# Testing the Copr consumer locally

With the compose services up and `copr_ingest.enabled: true` in `configs/config.yaml`, these fixtures cover
successful builds, invalid messages, and one skipped event type. Only `rhel-*` chroots create
repositories for now (`CoprChrootDistros` in `pkg/config/value_constraints.go`). A repository is named after
the Copr project (with its owner) and chroot, for example `@nvidia/cuda for RHEL 9 x86_64`.

`scripts/create_copr_repo.sh` simulates two Copr builds in Pulp, in the app's Copr domain (`clients.pulp.copr_domain`),
and writes copies of `build-end.json` and `build-end-second-build.json` with real hrefs from that domain into
`generated/` (gitignored). Send those copies, not the committed ones. Re-running it is safe. The other fixtures keep fake hrefs, since they're skipped before Pulp is accessed. Send `build-end.json` before
`build-end-second-build.json`; sending either again won't create another snapshot for that repository version.
Skipped messages are committed too, so check the logs to confirm the result.

| File | Expected result |
|------|-----------------|
| `generated/build-end.json` | Creates the repository and its first snapshot for the Copr org (`clients.pulp.copr_org_id`, `-5` by default) |
| `generated/build-end-second-build.json` | Adds a second snapshot to the same repository |
| `build-end-missing-fields.json` | No task; logs `skipping invalid message` |
| `build-end-data-only.json` | No task; logs `skipping invalid message` |
| `build-end-unexpected-type.json` | No task; logs `skipping event type` at debug level |

## Run the fixtures

From the repository root, set up Pulp in one terminal:

```sh
./scripts/create_copr_repo.sh
```

In a second terminal, start the consumer and task worker:

```sh
go run ./cmd/content-sources copr-ingest task-worker
```

Send a fixture (the key makes it easy to find), then inspect the topic and consumer offset:

```sh
make kafka-produce-message KAFKA_TOPIC=copr.build.events \
  KAFKA_MESSAGE_FILE=test/kafka/copr/generated/build-end.json KAFKA_MESSAGE_KEY=test-1

# Check that it's in the topic (Ctrl-C to stop)
make kafka-topic-consume KAFKA_TOPIC=copr.build.events

# Check that the consumer committed it
make kafka-group-committed KAFKA_TOPIC=copr.build.events KAFKA_GROUP_ID=content-sources-copr
```

Check the app logs for task progress and skipped-message reasons, and the metrics for the result of each message:

```sh
curl -s localhost:9000/metrics | grep content_sources_copr_
```
