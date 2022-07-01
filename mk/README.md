# Makefile composition

The repository is using a way to compose the `Makefile` file which
empower the single responsability principal taken into the Makefile
file usage.

We have the following files:

```raw
Makefile                    # Minimal main Makefile
mk
├── README.md               # This documentation file
├── db.mk
├── docker.mk
├── go-rules.mk
├── help.mk                 # Rule to print out generated help content from the Makefile's files
├── includes.mk             # Entry point included into the main Makefile
├── meta-db.mk
├── meta-docker.mk
├── meta-general.mk
├── plantuml.mk
├── printvars.mk            # print out variables
├── projectdir.mk           # Store at PROJECT_DIR the base directory for the repository
├── swag.mk
└── variables.mk            # Default values to the project variables that has not been
                            # overrided by the environment variables nor by configs/config.yaml
                            # file
```

## Usage

- Print out help: `make help`
- Get dependencies: `make get-deps`
- Build binaries: `make build`
- Clean binaries: `make clean`
- Launch clean tests: `make db-clean clean build db-up test`
- Prepare database without the migrations: `make db-clean db-up RUN_MIGRATION=`
- Open the postgres cli in the current executing container: `make db-cli-connect`
- Populate database with random records: `make db-migrate-seed`
- Build the docker container image: `make docker-build`
- Push the docker image to the registry:

  ```sh
  make docker-build docker-push
  ```

  > By default it uses the local username as the `QUAY_USER`. Can be overrided.
  > It uses the image name that is published by default.
  > It tag the image with the current git short hash.
  > We can publish in our personal quay.io account, deploy in ephemeral
  > environment the fresh image container, and see if the image behave
  > as we expect before to push our changes to the repository.

  ```sh
  make docker-build docker-push DOCKER_IMAGE="quay.io/my-user/my-image:my-tag"
  ```

  ```sh
  make docker-build docker-push QUAY_USER="my-user"
  ```

  ```sh
  make docker-build docker-push DOCKER_IMAGE_BASE="quay.io/my-user/my-image"
  ```

  > See `make printvars` to check related variables.

## More detailed explanation

It uses single responsability principal and split basic or logical groupable
rules in a single file.

We could group into the following way:

@Minimal and layout composition

- `Makefile`: Entrypoint and the only responsability is to include
  the list of includes and set the dafault rule.
- `mk/includes.mk`: This define the composition for your repository
  and it reference other `mk/*` files, and the `mk/meta-*` files
  which allow the `help` rule to print out the group title in the
  right position.

@Variables

- `mk/projectdir.mk`: It uses the first include and stores
  in the `PROJECT_DIR` variable the path to the repository in the
  file system.
- `mk/variables.mk`: It defines the default values for variables.

@Help infrastructure

- `mk/meta-*.mk`: This file is just one line of content for each one,
  but that allow us to print the group label into the help generated
  as we want. It can not be written directly into the `mk/includes.mk`
  file because of the order the files are processed when the `help`
  rule is invoked.
- `mk/help.mk`: Contains the rule that generates the help content.

@Rules

- `mk/go-rules.mk`: Provide useful rules to build, clean, and test
  a golang project. It provides generic rules to build any
  `cmd/COMMAND` directory into the `OUTPUT` directory putting
  there the generated binary. So if we add a new command, we
  don't need to change anything in the Makefile; the binary
  generated name is the COMMAND directory name. Example, if I
  have the file `cmd/nice-command/main.go`, when I invoke the
  `build` rule I will get the `release/nice-command` binary
  if the compilation is succesful, and no changes are needed
  to the Makefile if a new `cmd/other-nice-command/main.go`
  is created. "Write once, use forever".
- `mk/docker.mk`: Generic rules to build and push container images.
  It has an interesting set of variables that combined properly
  allow us a lot of customization and create new rules on them.
- `mk/db.mk`: Rules to start and stop a postgres container database
  and clean the data volume. It adds some other rules to quickly
  invoke the migration and seed the database with random values.
- `mk/plantuml.mk`: Invoke plantuml to generate diagrams in SVG
  format from the `docs/*.puml` files. It requires that plantuml
  is installed in your environment. Add a generic rule for it, so
  that no changes are needed when new `.puml` file is added to the
  `doc/` directory.

@Miscelanea

- `mk/printvars.mk`: It helps to fix wrong behaviors so it print out
  the variable name and the value not evaluated, so we can realize
  what final values could be applied and how should the values be
  expanded.

## Contributing

Do you have a rule or some set of rules that could be useful here?
Do not hesitate to contribute and review them together. It does
not matter how big is the rule, "big things are built of huge small changes".
