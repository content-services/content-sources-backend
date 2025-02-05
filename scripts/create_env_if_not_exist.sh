#!/bin/bash

# Define the paths
ENV_PATH="./_playwright-tests/.env"
EXAMPLE_ENV_PATH="./_playwright-tests/example.env"

# Check if the .env file exists
if [ ! -f "$ENV_PATH" ]; then
  # If it doesn't exist, copy the example.env file and rename it to .env
  cp "$EXAMPLE_ENV_PATH" "$ENV_PATH"
  echo "Copied $EXAMPLE_ENV_PATH to $ENV_PATH"
else
  echo "$ENV_PATH already exists"
fi