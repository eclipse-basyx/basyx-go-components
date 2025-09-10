#!/bin/sh

# Get the server port from environment or use default
PORT=${SERVER_PORT:-5000}
# Get the context path from environment or use default
CONTEXT_PATH=${SERVER_CONTEXTPATH:-}

# Construct the health check URL
if [ -z "$CONTEXT_PATH" ]; then
    HEALTH_URL="http://localhost:$PORT/health"
else
    HEALTH_URL="http://localhost:$PORT$CONTEXT_PATH/health"
fi

# Perform health check
wget --spider -q "$HEALTH_URL"
