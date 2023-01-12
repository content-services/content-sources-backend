##
# This file contains the default variable values
# that will be used along the Makefile execution.
#
# The values on this file can be overrided from
# the 'mk/private.mk' file (which is ignored by
# .gitignore file), that can be created from the
# 'mk/private.mk.example' file. It is recommended
# to use conditional assignment in your private.mk
# file, so that you can override values from
# the environment variable or just assigning the
# variable value when invoking 'make' command.
##


# The directory where golang will output the built binaries
GO_OUTPUT ?= $(PROJECT_DIR)/release

# Path to the configuration file
CONFIG_YAML := $(PROJECT_DIR)/configs/config.yaml

# The name of application
APP_NAME=content-sources

# The name for this component (e.g.: backend, frontend)
APP_COMPONENT=backend

#
# Database variables (expected from configs/config.yaml)
#

# Here just indicate that variables should be exported
LOAD_DB_CFG_WITH_YQ := n
ifneq (,$(shell yq --version 2>/dev/null))
ifneq (,$(shell ls -1 "$(CONFIG_YAML)" 2>/dev/null))
LOAD_DB_CFG_WITH_YQ := y
endif
endif

COMPOSE_PROJECT_NAME ?= cs
export COMPOSE_PROJECT_NAME

DATABASE_CONTAINER_NAME=$(COMPOSE_PROJECT_NAME)_postgres-content_1
ifeq (y,$(LOAD_DB_CFG_WITH_YQ))
$(info info:Trying to load DATABASE configuration from '$(CONFIG_YAML)')
DATABASE_HOST ?= $(shell yq -r -M '.database.host' "$(CONFIG_YAML)")
DATABASE_EXTERNAL_PORT ?= $(shell yq -M '.database.port' "$(CONFIG_YAML)")
DATABASE_INTERNAL_PORT ?= 5432
DATABASE_NAME ?= $(shell yq -r -M '.database.name' "$(CONFIG_YAML)")
DATABASE_USER ?= $(shell yq -r -M '.database.user' "$(CONFIG_YAML)")
DATABASE_PASSWORD ?= $(shell yq -r -M '.database.password' "$(CONFIG_YAML)")
else
$(info info:Using DATABASE_* defaults)
DATABASE_HOST ?= localhost
DATABASE_INTERNAL_PORT ?= 5432
DATABASE_EXTERNAL_PORT ?= 5433
DATABASE_NAME ?= content
DATABASE_USER ?= content
DATABASE_PASSWORD ?= content
endif
LOAD_DB_CFG_WITH_YQ :=

# Make the values availables for the forked processes as env vars
export DATABASE_CONTAINER_NAME
export DATABASE_HOST
export DATABASE_NAME
export DATABASE_USER
export DATABASE_INTERNAL_PORT #Internal to the container
export DATABASE_EXTERNAL_PORT #External to the container on localhost
export DATABASE_DATA_DIR


DEPLOY_PULP ?= "false"
export DEPLOY_PULP

#
# Container variables
#

# Default QUAY_USER set to the current user
# Customize it at 'mk/private.mk' file
QUAY_USER ?= $(USER)
# Context directory to be used as base for building the container
DOCKER_CONTEXT_DIR ?= .
# Path to the main project Dockerfile file
DOCKER_DOCKERFILE ?= build/Dockerfile
# Base image name
DOCKER_IMAGE_BASE ?= quay.io/$(QUAY_USER)/content-sources
# Default image tag is set to the short git hash of the repo
DOCKER_IMAGE_TAG ?= $(shell git rev-parse --short HEAD)
# Compose the container image with all the above
DOCKER_IMAGE ?= $(DOCKER_IMAGE_BASE):$(DOCKER_IMAGE_TAG)

#
# Kafka configuration variables
#

# The directory where the kafka data will be stored
KAFKA_DATA_DIR ?= $(PROJECT_DIR)/compose_files/kafka/data

# The directory where the kafka configuration will be
# bound to the containers
KAFKA_CONFIG_DIR ?= $(PROJECT_DIR)/compose_files/kafka/config

# The topics used by the repository
# Updated to follow the pattern used at playbook-dispatcher
KAFKA_TOPICS ?= platform.content-sources.introspect

# The group id for the consumers; every consumer subscribed to
# a topic with different group-id will receive a copy of the
# message. In our scenario, any replica of the consumer wants
# only one message to be processed, so we only use a unique
# group id at the moment.
KAFKA_GROUP_ID ?= content-sources

# Read the last kafka version
KAFKA_VERSION ?= $(shell $(PROJECT_DIR)/scripts/kafka-print-last-version.py)


# Set OPEN command
ifneq (,$(shell command -v xdg-open 2>/dev/null))
OPEN ?= xdg-open
endif

ifneq (,$(shell command -v open 2>/dev/null))
OPEN ?= open
endif


# Set default metrics configuration
METRICS_PATH ?= /metrics
METRICS_PORT ?= 9000
export METRICS_PATH
export METRICS_PORT
