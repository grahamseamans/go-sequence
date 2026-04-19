#!/bin/bash
# Sets up the coworker (Grok) MCP server inside the devcontainer.
#
# Why this exists: the coworker MCP is a local Node.js project on the host
# Mac. The devcontainer has no access to the host Mac filesystem outside the
# workspace mount, so we can't just point Claude at the host's path.
#
# This script copies the MCP source from the host into the container's
# persistent `~/.claude` volume, installs runtime deps, and registers the
# MCP with Claude Code. Idempotent — safe to re-run after rebuild.
#
# Run FROM THE HOST MAC (not inside the container):
#     ./.devcontainer/setup-coworker-mcp.sh
#
# Assumes the devcontainer is already running for this repo.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST_COWORKER_DIR="/Users/cibo/code/coworker_mcp"
CONTAINER_DEST="/home/node/.claude/mcp-servers/coworker_mcp"

# Find the running container for this repo
CONTAINER=$(docker ps -q -f "label=devcontainer.local_folder=$REPO_ROOT")
if [ -z "$CONTAINER" ]; then
    echo "ERROR: No running devcontainer for $REPO_ROOT. Start it first." >&2
    exit 1
fi

if [ ! -d "$HOST_COWORKER_DIR" ]; then
    echo "ERROR: $HOST_COWORKER_DIR not found on host. Cannot set up." >&2
    exit 1
fi

# Extract XAI_API_KEY from host's Claude config.
XAI_KEY=$(python3 -c "import json; d=json.load(open('/Users/cibo/.claude.json')); print(d['mcpServers']['coworker']['env']['XAI_API_KEY'])")
if [ -z "$XAI_KEY" ]; then
    echo "ERROR: Could not read XAI_API_KEY from host's ~/.claude.json." >&2
    exit 1
fi

echo "→ Copying coworker MCP source into container..."
docker exec --user root "$CONTAINER" mkdir -p "$CONTAINER_DEST"
docker cp "$HOST_COWORKER_DIR/dist" "$CONTAINER:$CONTAINER_DEST/"
docker cp "$HOST_COWORKER_DIR/package.json" "$CONTAINER:$CONTAINER_DEST/"
docker cp "$HOST_COWORKER_DIR/package-lock.json" "$CONTAINER:$CONTAINER_DEST/"
docker exec --user root "$CONTAINER" chown -R node:node "$CONTAINER_DEST"

echo "→ Installing runtime deps..."
docker exec --user node "$CONTAINER" sh -c "cd $CONTAINER_DEST && npm install --omit=dev --silent"

echo "→ Registering MCP (removing old entry if present)..."
docker exec --user node "$CONTAINER" claude mcp remove coworker 2>/dev/null || true
docker exec --user node -e XK="$XAI_KEY" "$CONTAINER" sh -c \
    "claude mcp add coworker --env XAI_API_KEY=\"\$XK\" -- node $CONTAINER_DEST/dist/index.js" >/dev/null

echo "→ Verifying connection..."
docker exec --user node "$CONTAINER" claude mcp list 2>&1 | grep coworker

echo "Done."
