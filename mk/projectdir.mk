##
# Small file which only assign the path of the project by
# reading the absolute path for the main Makefile.
##
PROJECT_DIR := $(shell dirname $(abspath $(firstword $(MAKEFILE_LIST))))
