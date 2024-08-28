# Content Sources

## What is it?

Content Sources is an application for storing information about external content (currently YUM repositories) in a central location as well as creating snapshots of those repositories, backed by a Pulp server.

To read more about Content Sources use cases see:
1. [Introspection](./docs/workflows/introspection.md)
2. [Snapshots](./docs/workflows/snapshotting.md)


## Developing

### Requirements:

1. podman & podman-compose installed or docker & docker-compose installed (and docker running)
   - This is used to start a set of containers that are dependencies for content-sources-backend
2. yaml2json tool installed (`pip install json2yaml`).

### Create your configuration

Create a config file from the example:

```sh
$ cp ./configs/config.yaml.example ./configs/config.yaml
```

### Import Public Repos

```sh
$ make repos-import
```

### Start dependency containers

```sh
$ make compose-up
```

### Run the server!

```sh
$ make run
```

###

Hit the API:

```sh
  $ curl -H "$( ./scripts/header.sh 9999 1111 )" http://localhost:8000/api/content-sources/v1.0/repositories/
```

### Stop dependency containers

When its time to shut down the running containers:

```sh
$ make compose-down
```

And clean the volume that it uses by (this stops the container before doing it if it were running):

```sh
$ make compose-clean
```

> There are other make rules that could be helpful, run `make help` to list them. Some are highlighted below

### HOW TO ADD NEW MIGRATION FILES

You can add new migration files, with the prefixed date attached to the file name, by running the following:

```
$ go run cmd/dbmigrate/main.go new <name of migration>
```

### Database Commands

Migrate the Database

```sh
$ make db-migrate-up
```

Seed the database

```sh
$ make db-migrate-seed
```

Get an interactive shell:

```sh
$ make db-shell
```

Or open directly a postgres client by running:

```sh
$ make db-cli-connect
```

### Kafka commands

You can open an interactive shell by:

```sh
$ make kafka-shell
```

You can run kafka-console-consumer.sh using `KAFKA_TOPIC` by:

```sh
$ make kafka-topic-consume KAFKA_TOPIC=my-kafka-topic
$ make kafka-topic-consume # Use the first topic at KAFKA_TOPICS list
```

> There are other make rules that could be helpful,
> run `make help` to list them.

### Start / Stop prometheus

Create the configuration for prometheus, getting started with the example one.

Update the `configs/prometheus.yaml` file to set your hostname instead of `localhost` at `scrape_configs.job_name.targets`:

```sh
# Note that the targets object cannot reference localhost, it needs the name of your host where
# the prometheus container is executed.
$ cat ./configs/prometheus.example.yaml | sed "s/localhost/$(hostname)/g" > ./configs/prometheus.yaml
```

To start prometheus run:

```sh
$ make prometheus-up
```

To stop prometheus container run:

```sh
$ make prometheus-down
```

To open the prometheus web UI, once the container is up, run the below:

```sh
$ make prometheus-ui
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
  ```

**Running it**

- Run the application by: `make run` or `./release/content-sources api consumer instrumentation mock_rbac`.
- Make some request using: `./scripts/header.sh 12345 jdoe@example.com` for admin or `./scripts/header.sh 12345 tdoe@example.com` for viewer.

> RBAC mock service is started for `make run`
> To use it running directly the service: `./release/content-sources api consumer instrumentation mock_rbac`
> Add the option `mock_rbac`

### Migrate your database (and seed it if desired)

```sh
$ make db-migrate-up
```

```sh
$ make db-migrate-seed
```

### Run the server!

```sh
$ make run
```

###

Hit the API:

```sh
$ curl -H "$( ./scripts/header.sh 9999 1111 )" http://localhost:8000/api/content-sources/v1.0/repositories/
```

### Generating new openapi docs:

```sh
$ make openapi
```

### Generating new mocks:

```sh
$ make mock
```

### Live Reloading Server
This is completely optional way of running the server that is useful for local development. It rebuilds the project after every change you make, so you always have the most up-to-date server running.
To set this up, all you need to do is install the "Air" go tool, [here is how](https://github.com/air-verse/air?tab=readme-ov-file#installation).
The recommended way is doing:
```sh
$ go install github.com/air-verse/air@latest
```

After that, all that needs to be done is just running `air`, it should automatically use the defined config for this project ([.air.toml](.air.toml)).
```sh
$ air
```

### Configuration

The default configuration file in ./configs/config.yaml.example shows all available config options. Any of these can be overridden with an environment variable. For example "database.name" can be passed in via an environment variable named "DATABASE_NAME".

### Linting

To use golangci-lint:

1. `make install-golangci-lint`
2. `make lint`

To use pre-commit linter: `make install-pre-commit`

### Code Layout

| Path                                           | Description                                                                                                                                                                                   |
|------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [api](./api/)                                  | Openapi docs and doc generation code                                                                                                                                                          |
| [db/migrations](./db/migrations/)              | Database Migrations                                                                                                                                                                           |     |
| [pkg/api](./pkg/api)                           | API Structures that are used for handling data within our API Handlers                                                                                                                        |
| [pkg/config](./pkg/config)                     | Config loading and application bootstrapping code                                                                                                                                             |
| [pkg/dao](./pkg/dao)                           | Database Access Object. Abstraction layer that provides an interface and implements it for our default database provider (postgresql). It is separated out for abstraction and easier testing |
| [pkg/db](./pkg/db)                             | Database connection and migration related code                                                                                                                                                |
| [pkg/handler](./pkg/handler)                   | Methods that directly handle API requests                                                                                                                                                     |
| [pkg/middleware](./pkg/middleware)             | Holds all the middleware components created for the service                                                                                                                                   |
| [pkg/event](./pkg/event)                       | Event message logic. More info [here](./pkg/event/README.md)                                                                                                                                  |
| [pkg/models](./pkg/models)                     | Structs that represent database models (Gorm)                                                                                                                                                 |
| [pkg/seeds](./pkg/seeds)                       | Code to help seed the database for both development and testing                                                                                                                               |
| [pkg/candlepin_client](./pkg/candlepin_client) | Candlepin client                                                                                                                                                                              |
| [pkg/pulp_client](./pkg/pulp_client)           | Pulp client                                                                                                                                                                                   |
| [pkg/tasks](./pkg/tasks)                       | Tasking system. More info [here](./docs/tasking_system/tasking_system.md)                                                                                                                     |
| [scripts](./scripts)                           | Helper scripts for identity header generation and testing                                                                                                                                     |


## More info

- [Architecture](docs/architecture.md)
- [OpenApi Docs](https://redocly.github.io/redoc/?url=https://raw.githubusercontent.com/content-services/content-sources-backend/main/api/openapi.json)
