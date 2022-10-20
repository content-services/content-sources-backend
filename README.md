# Content Sources

## What is it?
Content Sources is an application for storing information about external content (currently YUM repositories) in a central location.


## Developing

### Requirements:

1. podman installed or docker installed (and running)

### Create your configuration

Create a config file from the example:

```sh
$ cp ./configs/config.yaml.example ./configs/config.yaml
```

### Start / Stop postgres

- Start the database container by:

  ```sh
  $ make db-up
  ```

---

- You can stop it by:

  ```sh
  $ make db-down
  ```

- And clean the volume that it uses by (this stop
  the container before doing it if it were running):

  ```sh
  $ make db-clean
  ```

- Or inspect inside the container if necessary meanwhile it is
  running by:

  ```sh
  $ make db-shell
  ```

- Or open directly a postgres client by running:

  ```sh
  $ make db-cli
  ```

### Start / Stop kafka

- Build the kafka container (only once):

  ```sh
  $ make kafka-build
  ```

- Start the kafka containers by:

  ```sh
  $ make kafka-up
  ```

---

- You can stop it by:

  ```sh
  $ make kafka-down
  ```

- You can clean the kafka instance by:

  ```sh
  # This remove kafka/data directory
  $ make kafka-clean
  ```

> The kafka configuration is located at `kafka/config` if you need
> to customize some configuration.

- You can open an interactive shell by:

  ```sh
  $ make kafka-shell
  ```

- You can run kafka-console-consumer.sh using `KAFKA_TOPIC` by:

  ```sh
  $ make kafka-topic-consume KAFKA_TOPIC=my-kafka-topic
  $ make kafka-topic-consume # Use the first topic at KAFKA_TOPICS list
  ```

> There are other make rules that could be helpful, just
> run `make help` to check them.

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

Hit the api:

```sh
$ curl -H "$( ./scripts/header.sh 9999 1111 )" http://localhost:8000/api/content-sources/v1.0/repositories/
```

### Generating new openapi docs:

```sh
$ make openapi
```

### Configuration

The default configuration file in ./configs/config.yaml.example shows all available config options.  Any of these can be overridden with an environment variable.  For example  "database.name" can be passed in via an environment variable named "DATABASE_NAME".

### Linting

To use golangci-lint:
1. `make install-golangci-lint`
2. `make lint`

To use pre-commit linter: `make install-pre-commit`

### Code Layout

| Path                              | Description                                                                                                                                                                                     |
|-----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [api](./api/)                     | Openapi docs and doc generation code                                                                                                                                                            |
| [db/migrations](./db/migrations/) | Database Migrations                                                                                                                                                                             |                                                                                                                                                                            |
| [pkg/api](./pkg/api)              | API Structures that are used for handling data within our API Handlers                                                                                                                          |
| [pkg/config](./pkg/config)        | Config loading and application bootstrapping code                                                                                                                                               |
| [pkg/dao](./pkg/dao)              | Database Access Object.  Abstraction layer that provides an interface and implements it for our default database provider (postgresql).  It is separated out for abstraction and easier testing |
| [pkg/db](./pkg/db)                | Database connection and migration related code                                                                                                                                                  |
| [pkg/handler](./pkg/handler)      | Methods that directly handle API requests                                                                                                                                                       |
| [pkg/models](./pkg/models)        | Structs that represent database models (Gorm)                                                                                                                                                   |
| [pkg/seeds](./pkg/seeds)          | Code to help seed the database for both development and testing                                                                                                                                 |


### Contributing

 * Pull requests welcome!
 * Pull requests should come with good tests
 * All PRs should be backed by a JIRA ticket and included in the subject using the format:
   * `Fixes 23: Some great feature` - if a PR fully resolves an issue or feature, or is the last PR in a series of PRs.
   * `Refs 23: Partial work for issue` - if a PR partially resolves an issue or feature or is not the last PR in a series of PRs.
   * If you do not have access to the Jira (we are working on opening it up), open a PR without this
     and we will add it for you.

## More info

 * [Architecture](docs/architecture.md)
 * [OpenApi Docs](https://redocly.github.io/redoc/?url=https://raw.githubusercontent.com/content-services/content-sources-backend/main/api/openapi.json)
