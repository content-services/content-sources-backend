# Content Sources

## What is it?

Content Sources is an application for storing information about external content (currently YUM repositories) in a central location as well as creating snapshots of those repositories, backed by a Pulp server.

To read more about Content Sources use cases see:

1. [Introspection](./docs/workflows/introspection.md)
2. [Snapshots](./docs/workflows/snapshotting.md)

## Developing

### Requirements:

1. podman & podman-compose [Do not use v1.3.0](https://github.com/containers/podman-compose/issues/1109), or docker & docker-compose, installed and running (and Orbstack for Mac))
   - This is used to start a set of containers that are dependencies for content-sources-backend
   - Alternatively docker and docker-compose is supported
2. yaml2json tool installed (`pip install json2yaml`).

### Create your configuration

Create a config file from the example:

```sh
cp ./configs/config.yaml.example ./configs/config.yaml
```

### Add pulp.content to /etc/hosts for Go integration tests and client access

```sh
sudo echo "<your-host-ip> pulp.content" | sudo tee -a /etc/hosts
```

You can find your host IP by running `ifconfig` or `ip addr list`. 
If you have a virtual bridge interface (virbr0, started up by running `virt-manager`), then you can use that IP.

### Start dependency containers

```sh
make compose-up
```

If you want to start containers without initializing any data:

```sh
make compose-run
```

### Import RHEL 9 Repos

```sh
make repos-import-rhel9
```
or import them all
```sh
make repos-import
```

### Introspect and Snapshot all current repos
```sh
make process-repos
```

### For local development

If you want less Red Hat repos:

```sh
OPTIONS_REPOSITORY_IMPORT_FILTER=small make repos-import
```

This will import and snapshot repos needed for the minimal viable environment. Useful for running Playwright tests.

```sh
make repos-minimal
```

### Run the server!

```sh
make run
```

###

Hit the API:

```sh
curl -H "$( ./scripts/header.sh 9999 1111 )" http://localhost:8000/api/content-sources/v1.0/repositories/
```

### Stop dependency containers

When its time to shut down the running containers:

```sh
make compose-down
```

Clean the volume that it uses (this stops the container before doing it if it were running):

```sh
make compose-clean
```

> There are other make rules that could be helpful, run `make help` to list them. Some are highlighted below

## Playwright testing

- Ensure that the backend server is running
- Ensure the correct [node version](_playwright-tests/.nvmrc), is installed and in use:  
  `cd _playwright-tests` and `nvm use`
- Copy the [env](_playwright-tests/example.env) file and create a file at: \_playwright-tests/.env
  For local development only the BASE_URL:`http://127.0.0.1:8000` is required, which is already set in the example config.

```sh
make playwright
```

OR

```sh
cd _playwright-tests \
&& yarn install \
&& yarn playwright install \
&& yarn playwright test
```

### HOW TO ADD NEW MIGRATION FILES

You can add new migration files, with the prefixed date attached to the file name, by running the following:

```
go run cmd/dbmigrate/main.go new <name of migration>
```

### Database Commands

Migrate the Database

```sh
make db-migrate-up
```

Get an interactive shell:

```sh
make db-shell
```

Or open directly a postgres client by running:

```sh
make db-cli-connect
```

### Kafka commands

You can open an interactive shell by:

```sh
make kafka-shell
```

You can run kafka-console-consumer.sh using `KAFKA_TOPIC` by:

```sh
make kafka-topic-consume KAFKA_TOPIC=my-kafka-topic
make kafka-topic-consume # Use the first topic at KAFKA_TOPICS list
```

> There are other make rules that could be helpful,
> run `make help` to list them.

### Start / Stop prometheus

Create the configuration for prometheus, getting started with the example one.

Update the `configs/prometheus.yaml` file to set your hostname instead of `localhost` at `scrape_configs.job_name.targets`:

```sh
# Note that the targets object cannot reference localhost, it needs the name of your host where
# the prometheus container is executed.
cat ./configs/prometheus.example.yaml | sed "s/localhost/$(hostname)/g" > ./configs/prometheus.yaml
```

To start prometheus run:

```sh
make prometheus-up
```

To stop prometheus container run:

```sh
make prometheus-down
```

To open the prometheus web UI, once the container is up, run the below:

```sh
make prometheus-ui
```

### Start / Stop mock for rbac

**Configuration requirements**

- To use this you need to enable RBAC into `config/configs.yaml` file:

  ```yaml
  clients:
    rbac_enabled: True
    rbac_base_url: http://localhost:8800/api/rbac/v1
    rbac_timeout: 30
  mocks:
    rbac:
      user_read_write: ["jdoe@example.com", "jdoe"]
      user_read: ["tdoe@example.com", "tdoe"]
    kessel:
      user_read_write: ["write-user"]
      user_read: ["read-user"]
      user_no_permissions: ["no-perms-user"]
  ```

**Running it**

- Run the application by: `make run` or `./release/content-sources api consumer instrumentation mock_rbac`.
- Make some request using: `./scripts/header.sh 12345 jdoe@example.com` for admin or `./scripts/header.sh 12345 tdoe@example.com` for viewer.

> RBAC mock service is started for `make run`
> To use it running directly the service: `./release/content-sources api consumer instrumentation mock_rbac`
> Add the option `mock_rbac`

**Kessel**
- The kessel permissions will be used if the kessel feature is enabled.
- Kessel will be used alongside rbac when only enabled for specific users, orgs, or accounts.


### Migrate your database (and seed it if desired)

```sh
make db-migrate-up
```

### Run the server!

```sh
make run
```

###

Hit the API:

```sh
curl -H "$( ./scripts/header.sh 9999 1111 )" http://localhost:8000/api/content-sources/v1.0/repositories/
```

### Generating new openapi docs:

```sh
make openapi
```

### Generating new mocks:

```sh
make mock
```

### Live Reloading Server

This is completely optional way of running the server that is useful for local development. It rebuilds the project after every change you make, so you always have the most up-to-date server running.
To set this up, all you need to do is install the "Air" go tool, [here is how](https://github.com/air-verse/air?tab=readme-ov-file#installation).
The recommended way is doing:

```sh
go install github.com/air-verse/air@latest
```

After that, all that needs to be done is just running `air`, it should automatically use the defined config for this project ([.air.toml](.air.toml)).

```sh
air
```

### Testing with Image Builder Frontend

While working on features in the Image Builder frontend that integrate with this service, it can be useful to develop and test those alongside each other locally.
To do that there are 2 changes required in the IB FE repo:

1. Changing the path to the content sources API config. From which will Redux generate the clients. \
   `api/config/contentSources.ts`: `schemaFile: '/YOUR_LOCAL_PATH_TO_THIS_REPO/content-sources-backend/api/openapi.json',`
2. Adding a route to the FEC config, which will be picked by the frontend dev proxy. \
   `fec.config.js`: `['/api/content-sources']: { host: 'http://127.0.0.1:8000' }`

<details>

  <summary>Example Git Patch:</summary>

```diff
diff --git a/api/config/contentSources.ts b/api/config/contentSources.ts
index 7d4db495..32a00e3f 100644
--- a/api/config/contentSources.ts
+++ b/api/config/contentSources.ts
@@ -1,7 +1,7 @@
 import type { ConfigFile } from '@rtk-query/codegen-openapi';

 const config: ConfigFile = {
-  schemaFile: 'https://console.redhat.com/api/content-sources/v1/openapi.json',
+  schemaFile: 'YOUR_LOCAL_PATH_TO_THE_CONTENT_SOURCES_BACKEND_REPO/content-sources-backend/api/openapi.json',
   apiFile: '../../src/store/service/emptyContentSourcesApi.ts',
   apiImport: 'emptyContentSourcesApi',
   outputFile: '../../src/store/service/contentSourcesApi.ts',
diff --git a/fec.config.js b/fec.config.js
index 3767689d..cba37234 100644
--- a/fec.config.js
+++ b/fec.config.js
@@ -102,6 +102,7 @@ module.exports = {
         };
       }, {}),
     }),
+    ['/api/content-sources']: { host: 'http://127.0.0.1:8000' }
   },
   plugins: plugins,
   moduleFederation: {
```

</details>

### Configuration

The default configuration file in ./configs/config.yaml.example shows all available config options. Any of these can be overridden with an environment variable. For example "database.name" can be passed in via an environment variable named "DATABASE_NAME".

### Linting

To use golangci-lint:

1. `make install-golangci-lint`
2. `make lint`

To use pre-commit linter: `make install-pre-commit`

### Code Layout

| Path                                             | Description                                                                                                                                                                                   |
| ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [api](./api/)                                   | Openapi docs and doc generation code                                                                                                                                                          |
| [db/migrations](./db/migrations/)               | Database Migrations                                                                                                                                                                           |
| [pkg/api](./pkg/api)                            | API Structures that are used for handling data within our API Handlers                                                                                                                        |
| [pkg/config](./pkg/config)                      | Config loading and application bootstrapping code                                                                                                                                             |
| [pkg/dao](./pkg/dao)                            | Database Access Object. Abstraction layer that provides an interface and implements it for our default database provider (postgresql). It is separated out for abstraction and easier testing |
| [pkg/db](./pkg/db)                              | Database connection and migration related code                                                                                                                                                |
| [pkg/handler](./pkg/handler)                    | Methods that directly handle API requests                                                                                                                                                     |
| [pkg/middleware](./pkg/middleware)              | Holds all the middleware components created for the service                                                                                                                                   |
| [pkg/event](./pkg/event/README.md)              | Event message logic. More info [here](./pkg/event/README.md)                                                                                                                                  |
| [pkg/models](./pkg/models)                      | Structs that represent database models (Gorm)                                                                                                                                                 |
| [pkg/seeds](./pkg/seeds)                        | Code to help seed the database for both development and testing                                                                                                                               |
| [pkg/candlepin_client](./pkg/clients/candlepin_client) | Candlepin client                                                                                                                                                                              |
| [pkg/pulp_client](./pkg/clients/pulp_client)           | Pulp client                                                                                                                                                                                   |
| [pkg/tasks](./pkg/tasks)                        | Tasking system. More info [here](./docs/tasking_system/tasking_system.md)                                                                                                                     |
| [scripts](./scripts)                            | Helper scripts for identity header generation and testing                                                                                                                                     |
## More info

- [Architecture](docs/architecture.md)
- [OpenApi Docs](https://redocly.github.io/redoc/?url=https://raw.githubusercontent.com/content-services/content-sources-backend/main/api/openapi.json)
