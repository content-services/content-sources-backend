#!/bin/bash
set -e

IMAGE="${IMAGE:-quay.io/cloudservices/content-sources-backend}"
IMAGE_TAG="$(git rev-parse --short=7 HEAD)"
SMOKE_TEST_TAG="latest"

if [[ -z "$QUAY_USER" || -z "$QUAY_TOKEN" ]]; then
    echo "QUAY_USER and QUAY_TOKEN must be set"
    exit 1
fi

if [[ -z "$RH_REGISTRY_USER" || -z "$RH_REGISTRY_TOKEN" ]]; then
    echo "RH_REGISTRY_USER and RH_REGISTRY_TOKEN  must be set"
    exit 1
fi

# Login to quay
make docker-login \
    DOCKER_LOGIN_USER="$QUAY_USER" \
    DOCKER_LOGIN_TOKEN="$QUAY_TOKEN" \
    DOCKER_REGISTRY="quay.io"

# Login to registry.redhat
make docker-login \
    DOCKER_LOGIN_USER="$RH_REGISTRY_USER" \
    DOCKER_LOGIN_TOKEN="$RH_REGISTRY_TOKEN" \
    DOCKER_REGISTRY="registry.redhat.io"

# build and push
make docker-build docker-push \
    DOCKER_BUILD_OPTS=--no-cache \
    DOCKER_IMAGE_BASE=$IMAGE \
    DOCKER_IMAGE_TAG=$IMAGE_TAG

# push to logged in registries and tag for SHA
docker tag "${IMAGE}:${IMAGE_TAG}" "${IMAGE}:${SMOKE_TEST_TAG}"
docker push "${IMAGE}:${SMOKE_TEST_TAG}"
