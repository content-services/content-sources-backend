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

CONFIG_YAML := $(PROJECT_DIR)/configs/config.yaml

## Database variables (expected from configs/config.yaml)
# Here just indicate that variables should be exported
LOAD_DB_CFG_WITH_YQ := n
ifneq (,$(shell yq --version 2>/dev/null))
ifneq (,$(shell ls -1 "$(CONFIG_YAML)" 2>/dev/null))
LOAD_DB_CFG_WITH_YQ := y
endif
endif

ifeq (y,$(LOAD_DB_CFG_WITH_YQ))
$(info info:Trying to load DATABASE configuration from '$(CONFIG_YAML)')
DATABASE_HOST ?= $(shell yq -r -M '.database.host' "$(CONFIG_YAML)")
DATABASE_PORT ?= $(shell yq -M '.database.port' "$(CONFIG_YAML)")
DATABASE_NAME ?= $(shell yq -r -M '.database.name' "$(CONFIG_YAML)")
DATABASE_USER ?= $(shell yq -r -M '.database.user' "$(CONFIG_YAML)")
DATABASE_PASSWORD ?= $(shell yq -r -M '.database.password' "$(CONFIG_YAML)")
else
$(info info:Using DATABASE_* defaults)
DATABASE_HOST ?= localhost
DATABASE_PORT ?= 5432
DATABASE_NAME ?= content
DATABASE_USER ?= content
DATABASE_PASSWORD ?= content
endif
override undefine LOAD_DB_CFG_WITH_YQ

# Make the values availables for the forked processes as env vars
export DATABASE_HOST
export DATABASE_PORT
export DATABASE_NAME
export DATABASE_USER
export DATABASE_PASSWORD


## Binary output
# The directory where golang will output the
# built binaries
GO_OUTPUT ?= $(PROJECT_DIR)/release


## Container variables
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

# KAFKA configurations
KAFKA_DATA_DIR ?= $(PROJECT_DIR)/kafka/data
KAFKA_CONFIG_DIR ?= $(PROJECT_DIR)/kafka/config
KAFKA_TOPICS ?= "repo-introspection"

