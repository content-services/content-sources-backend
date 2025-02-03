#!/bin/bash

# Function to check if the server is up
is_server_up() {
  response=$(curl --write-out '%{http_code}' --silent --output /dev/null http://127.0.0.1:8000/ping)
  if [ "$response" -eq 200 ]; then
    return 0
  else
    return 1
  fi
}

# Wait for the server to be ready with a timeout
echo "Waiting for backend to be ready..."
timeout=180  # Timeout in seconds
interval=1  # Check interval in seconds
elapsed=0

until is_server_up; do
 printf "\r%d/%d seconds have elapsed until timeout..." "$elapsed" "$timeout"
  sleep $interval
  elapsed=$((elapsed + interval))
  if [ $elapsed -ge $timeout ]; then
    echo -e "\nError: Timed out waiting for the backend server to be ready."
    exit 1
  fi
done

echo -e "\nBackend server is up and running after $elapsed seconds"