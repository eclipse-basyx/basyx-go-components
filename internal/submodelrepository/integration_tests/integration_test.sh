#!/bin/bash

# Integration Test Script
# Usage: ./integration_test.sh <config.json> [--no-rebuild]

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Usage: $0 <config.json> [--no-rebuild]"
    exit 1
fi

CONFIG_FILE=$1

if ! command -v jq &> /dev/null; then
    echo "jq is required but not installed. Please install jq."
    exit 1
fi

if ! command -v curl &> /dev/null; then
    echo "curl is required but not installed. Please install curl."
    exit 1
fi

# Stop and start Docker Compose
echo "Stopping Docker Compose..."
docker-compose -f docker_compose/docker_compose.yml down

# check if --no-rebuild is set
if [ "$2" = "--no-rebuild" ]; then
    echo "Starting Docker Compose without rebuild..."
    docker-compose -f docker_compose/docker_compose.yml up -d
else
    echo "Starting Docker Compose..."
    docker-compose -f docker_compose/docker_compose.yml up -d --build
fi


# Read the number of items in the config array
NUM_ITEMS=$(jq length "$CONFIG_FILE")

i=0
while [ $i -lt $NUM_ITEMS ]; do
    METHOD=$(jq -r ".[$i].method" "$CONFIG_FILE")
    ENDPOINT=$(jq -r ".[$i].endpoint" "$CONFIG_FILE")

    echo "Executing step $((i+1)): $METHOD to $ENDPOINT"

    if [ "$METHOD" = "POST" ]; then
        DATA_FILE=$(jq -r ".[$i].data" "$CONFIG_FILE")
        if [ ! -f "$DATA_FILE" ]; then
            echo "Data file $DATA_FILE not found."
            exit 1
        fi
        RESPONSE=$(curl -s -X POST -H "Content-Type: application/json" -d @"$DATA_FILE" "$ENDPOINT")
        echo "POST response: $RESPONSE"
    elif [ "$METHOD" = "GET" ]; then
        SHOULD_MATCH_FILE=$(jq -r ".[$i].data" "$CONFIG_FILE")
        if [ ! -f "$SHOULD_MATCH_FILE" ]; then
            echo "Should match file $SHOULD_MATCH_FILE not found."
            exit 1
        fi
        RESPONSE=$(curl -s "$ENDPOINT")
        echo "GET response: $RESPONSE"

        # Compare JSON
        EXPECTED=$(cat "$SHOULD_MATCH_FILE")
        if jq -e ". == $EXPECTED" <<< "$RESPONSE" > /dev/null; then
            echo "✓ Response matches expected."
        else
            echo "✗ Response does not match expected."
            echo "Expected: $EXPECTED"
            echo "Got: $RESPONSE"
            exit 1
        fi
    else
        echo "Unsupported method: $METHOD"
        exit 1
    fi
    i=$((i+1))
done

echo "All tests passed!"

# Stop Docker Compose
echo "Stopping Docker Compose..."
docker-compose -f docker_compose/docker_compose.yml down