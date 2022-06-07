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

## Database variables (expected from mk/private.mk or .env)
# Here just indicate that variables should be exported
export DATABASE_HOST
export DATABASE_PORT
export DATABASE_NAME
export DATABASE_USER
export DATABASE_PASSWORD

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

