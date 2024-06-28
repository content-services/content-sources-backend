#!/bin/bash

# --------------------------------------------
# Options that must be configured by app owner
# --------------------------------------------
APP_NAME="content-sources"                            # name of app-sre "application" folder this component lives in
COMPONENT_NAME="content-sources-backend"              # name of resourceTemplate component for deploy
IMAGE="quay.io/cloudservices/content-sources-backend" # image location on quay
DOCKERFILE="build/Dockerfile"

IQE_PLUGINS="content-sources"                       # name of the IQE plugin for this app.
IQE_MARKER_EXPRESSION="api"                         # This is the value passed to pytest -m
IQE_FILTER_EXPRESSION="not (introspection or rbac)" # This is the value passed to pytest -k
IQE_ENV="ephemeral"
IQE_CJI_TIMEOUT="30m" # This is the time to wait for smoke test to complete or fail
DEPLOY_TIMEOUT="900"  # 15min
REF_ENV="insights-stage"

COMPONENTS_W_RESOURCES="pulp"

# Only deploy one small red hat repo
EXTRA_DEPLOY_ARGS='--set-parameter content-sources-backend/OPTIONS_REPOSITORY_IMPORT_FILTER=small --set-parameter "content-sources-backend/NIGHTLY_CRON_JOB=5 4 25 12"'

# Install bonfire repo/initialize
# https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd/bootstrap.sh
# This script automates the install / config of bonfire
CICD_URL=https://raw.githubusercontent.com/RedHatInsights/bonfire/master/cicd
curl -s $CICD_URL/bootstrap.sh >.cicd_bootstrap.sh && source .cicd_bootstrap.sh

# This script is used to build the image that is used in the PR Check
source $CICD_ROOT/build.sh

# This script is used to deploy the ephemeral environment for smoke tests.
# The manual steps for this can be found in:
# https://consoledot.pages.redhat.com/docs/dev/operating-your-app/testing-iqe/testing.html#_deploy_ephemeral_env_sh_deploys_the_test_environment
source $CICD_ROOT/deploy_ephemeral_env.sh

# Run smoke tests using a ClowdJobInvocation and iqe-tests
source $CICD_ROOT/cji_smoke_test.sh

# Post a comment with test run IDs to the PR
source $CICD_ROOT/post_test_results.sh
