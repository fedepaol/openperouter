#!/bin/bash

# Script to wait for a binary to become available and then run it with arguments
# Usage: ./wait-and-run.sh <binary> [args...]

if [ $# -lt 1 ]; then
    echo "Usage: $0 <binary> [args...]" >&2
    exit 1
fi

BINARY="$1"
shift
ARGS="$@"

TIMEOUT=300  # 5 minutes in seconds
INTERVAL=1   # Check every 1 second
ELAPSED=0

echo "Waiting for binary '$BINARY' to become available..."

while [ $ELAPSED -lt $TIMEOUT ]; do
    if command -v "$BINARY" >/dev/null 2>&1; then
        echo "Binary '$BINARY' found! Executing: $BINARY $ARGS"
        exec "$BINARY" $ARGS
    fi

    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))

    # Progress indicator every 30 seconds
    if [ $((ELAPSED % 30)) -eq 0 ]; then
        echo "Still waiting... (${ELAPSED}s elapsed)"
    fi
done

echo "Timeout: Binary '$BINARY' not found after $TIMEOUT seconds" >&2
exit 1