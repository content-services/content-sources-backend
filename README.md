# Content Sources

## What is it?

Content Sources is an application for storing information about external content (currently YUM repositories) in a central location.


## Developing

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
  $ podman exec -it postgresql bash
  ```

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

```
curl http://localhost:8000/api/content_sources/v1.0/repositories/ ```
```

### Generating new openapi docs:

```sh
$ make openapi
```

### Configuration

The default configuration file in ./configs/config.yaml.example shows all available config options.  Any of these can be overridden with an environment variable.  For example  "database.name" can be passed in via an environment variable named "DATABASE_NAME".

### Contributing
 * Pull requests welcome!
 * Pull requests should come with good tests
 * Generally, feature PRs should be backed by a JIRA ticket and included in the subject using the format:
   * `CONTENT-23: Some great feature`

## More info
 * [Architecture](docs/architecture.md)
