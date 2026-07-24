#!/bin/bash

# Measure how long it takes to import, and snapshot or introspect particular repositories and wait until they become valid
# Run the backend locally

echo "------------------------------------------"
echo "Importing repos and forcing a snapshot..."
echo "------------------------------------------"

# # original - epel10 and small
# OPTIONS_REPOSITORY_IMPORT_FILTER=small go run ./cmd/external-repos/main.go import
# go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel9/9/aarch64/codeready-builder/os/ --force

# OPTIONS_REPOSITORY_IMPORT_FILTER=epel10 go run ./cmd/external-repos/main.go import
# go run cmd/external-repos/main.go snapshot --url https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/ --force
# # --

SECONDS=0

# update - hardcoded rhel 10, aarch: baseos and appstream
OPTIONS_REPOSITORY_IMPORT_FILTER=hardcoded go run ./cmd/external-repos/main.go import
go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel10/10/aarch64/appstream/os/ --force

OPTIONS_REPOSITORY_IMPORT_FILTER=hardcoded go run ./cmd/external-repos/main.go import
go run cmd/external-repos/main.go snapshot --url https://cdn.redhat.com/content/dist/rhel10/10/aarch64/baseos/os/ --force

OPTIONS_REPOSITORY_IMPORT_FILTER=epel10 go run ./cmd/external-repos/main.go import
go run cmd/external-repos/main.go introspect --url https://dl.fedoraproject.org/pub/epel/10/Everything/x86_64/ --force

echo "------------------------------------------"
echo "Repos imported."
echo "------------------------------------------"
echo "Checking snapshot status of all repos..."
echo "------------------------------------------"

while [[ "$(curl -s http://localhost:8000/api/content-sources/v1.0/repositories/ -H "$( ./scripts/header.sh 12675780 1111)" | jq '.data | all(.status == "Valid")')" == "false" ]]; do 
    echo "[$(date +%H:%M:%S)] Waiting... (${SECONDS}s elapsed)"
    sleep 5
done

duration=$SECONDS

echo "------------------------------------------"
echo "Success! All snapshots are Valid."
printf "%d min %d s\n" $((duration/60)) $((duration%60))
echo "${duration}s"
echo "------------------------------------------"