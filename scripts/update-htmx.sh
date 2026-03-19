#!/usr/bin/env bash
set -euo pipefail

DEST_DIR="ui/static/js"
DEST_FILE="$DEST_DIR/htmx.min.js"
URL="https://cdn.jsdelivr.net/npm/htmx.org@latest/dist/htmx.min.js"

mkdir -p "$DEST_DIR"

echo "Downloading latest htmx from $URL ..."
curl -fsSL "$URL" -o "$DEST_FILE"

echo "Updated htmx at $DEST_FILE"
