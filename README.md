# Content Sources

## What is it?
Content Sources is an application for storing information about external content (currently YUM repositories) in a central location.


## Developing

### Requirements:

1. podman & podman-compose installed or docker & docker-compose installed (and docker running)
   - This is used to start a set of containers that are dependencies for content-sources-backend
2. yaml2json tool installed (`pip install yaml2json`).

### Create your configuration

Create a config file from the example:

  ```sh
  $ cp ./configs/config.yaml.example ./configs/config.yaml
  ```

### Build needed kafka container

  ```sh
  $ make compose-build
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

> There are other make rules that could be helpful, run `make help` to list them.  Some are highlighted below


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

- To use this you need to enable RBAC into `config/configs.yaml` file:

  ```yaml
  clients:
    rbac_enabled: True
    rbac_base_url: http://localhost:8800/api/rbac/v1
    rbac_timeout: 30
  mocks:
    my_org_id: "12345"
    rbac:
      account_admin: "12345"
      account_viewer: "123456"
  ```

- Now run: `make mock-start`
- Run the application by: `make run`
- Make some request using: `./scripts/header.sh 12345 12345` for admin or `./scripts/header.sh 12345 123456` for viewer.
- When finished, stop rbac mock by: `make mock-stop`

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
| [pkg/middleware](./pkg/middleware)| Hold all the middleware components created for the service. |
| [pkg/event](./pkg/event)        | Event message logic. Mre info [here](./pkg/event/README.md). |
| [pkg/models](./pkg/models)        | Structs that represent database models (Gorm)                                                                                                                                                   |
| [pkg/seeds](./pkg/seeds)          | Code to help seed the database for both development and testing                                                                                                                                 |

## More info

 * [Architecture](docs/architecture.md)
 * [OpenApi Docs](https://redocly.github.io/redoc/?url=https://raw.githubusercontent.com/content-services/content-sources-backend/main/api/openapi.json)
