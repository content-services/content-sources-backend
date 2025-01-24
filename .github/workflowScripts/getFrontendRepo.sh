#!/bin/bash

# Check if the search string is provided as an argument
if [ -z "$1" ]; then
    echo "Usage: $0 <search-string>"
    exit 1
fi

# Variables
REPO_URL="https://api.github.com/repos/content-services/content-sources-frontend"
GIT_REPO_URL="https://github.com/content-services/content-sources-frontend.git"
CLONE_DIR=$(pwd)/content-sources-frontend
TAG_NAME="#testwith"
SEARCH_STRING="$1"

# Check if the folder exists
if [ -d "$CLONE_DIR" ]; then
    echo "Folder '$CLONE_DIR' exists. Removing it..."
    rm -rf "$CLONE_DIR"

    # Check if the removal was successful
    if [ ! -d "$CLONE_DIR" ]; then
        echo "Successfully removed the folder '$CLONE_DIR'."
    else
        echo "Failed to remove the folder '$CLONE_DIR'."
    fi
fi

# Create clone directory if it doesn't exist
mkdir -p $CLONE_DIR

# Fetch the list of open pull requests
prs=$(curl -s "$REPO_URL/pulls")

# Iterate through the list of PRs and clone the first matching PR
found_pr=false

while read -r pr; do
    # Extract PR title, body, and branch details
    pr_title=$(echo "$pr" | jq -r '.title')
    pr_body=$(echo "$pr" | jq -r '.body')
    pr_number=$(echo "$pr" | jq -r '.number')
    pr_branch=$(echo "$pr" | jq -r '.head.ref')
    pr_repo=$(echo "$pr" | jq -r '.head.repo.clone_url')

    # Check if PR title or body contains the search string
    if [[ "$pr_title" == *"$TAG_NAME $SEARCH_STRING"* ]] || [[ "$pr_body" == *"$TAG_NAME $SEARCH_STRING"* ]]; then
        echo "Cloning PR #$pr_number: $pr_title"
        git clone --branch $pr_branch $pr_repo $CLONE_DIR

        # Check if the clone was successful
        if [ $? -eq 0 ]; then
            found_pr=true
            echo "Successfully cloned PR #$pr_number into $CLONE_DIR"
        else
            echo "Failed to clone PR #$pr_number"
        fi
        
        # Exit the loop after cloning the first matching PR
        break
    fi
done < <(echo "$prs" | jq -c '.[]')

# If no matching PR was found, clone the main branch
if [ "$found_pr" == false ]; then
    echo "No PR title or description contains '$TAG_NAME $SEARCH_STRING'. Cloning the main branch."
    git clone --branch main $GIT_REPO_URL $CLONE_DIR/main

    # Check if the clone was successful
    if [ $? -eq 0 ]; then
        echo "Successfully cloned main branch into $CLONE_DIR/main"
    else
        echo "Failed to clone the main branch"
    fi
fi