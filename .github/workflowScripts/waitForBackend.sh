#!/bin/bash

# Start the backend server in the background
# echo "Starting backend server..."
# make run &

# Function to check if the server is up
is_server_up() {
  response=$(curl --write-out '%{http_code}' --silent --output /dev/null http://127.0.0.1:8000/ping)
  if [ "$response" -eq 200 ]; then
    return 0
  else
    return 1
  fi
}

# Wait for the server to be ready
echo "Waiting for backend to be ready..."
until is_server_up; do
  printf '.'
  sleep 5
done

echo "Backend server is up and running."