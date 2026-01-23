#!/bin/sh
PORT="${SERVER_PORT:-5004}"
CONTEXT_PATH="${SERVER_CONTEXTPATH:-}"

if [ -z "$CONTEXT_PATH" ]; then
  HEALTH_URL="http://127.0.0.1:${PORT}/health"
else
  HEALTH_URL="http://127.0.0.1:${PORT}${CONTEXT_PATH}/health"
fi

echo "Checking $HEALTH_URL"

# headers/status go to stderr -> redirect to stdout so Docker captures it
wget -qO- "$HEALTH_URL" >/dev/null
RESULT=$?

if [ "$RESULT" -ne 0 ]; then
  echo "Healthcheck failed for $HEALTH_URL with exit code $RESULT"
  exit "$RESULT"
fi

exit 0
