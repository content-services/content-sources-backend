#!/bin/bash

# Path to the go.mod file
GO_MOD_PATH=$1

# Check if the go.mod file exists
if [[ ! -f "$GO_MOD_PATH" ]]; then
    echo "go.mod file not found!"
    exit 1
fi

# Extract the Go version from the go.mod file
GO_VERSION=$(grep '^go ' "$GO_MOD_PATH" | awk '{print $2}')

# Check if the Go version was found
if [[ -z "$GO_VERSION" ]]; then
    echo "Go version not found in go.mod"
    exit 1
fi

# Print the Go version
echo "$GO_VERSION"