#!/usr/bin/env sh
set -eu

npm ci --prefix web/topology-app
npm run build --prefix web/topology-app
mkdir -p internal/mcp/ui
cp web/topology-app/dist/topology.html internal/mcp/ui/topology.html
