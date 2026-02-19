#!/usr/bin/env bash
set -euo pipefail

go install golang.org/x/tools/cmd/deadcode@latest
DEADCODE_BIN="$(go env GOPATH)/bin/deadcode"

# If GUI package is buildable in current environment, analyze full project roots.
if go test ./cmd/gui -run TestNonExistent -count=0 >/dev/null 2>&1; then
  "${DEADCODE_BIN}" ./cmd/... ./internal/...
  exit 0
fi

# Fallback for environments without CGO/Fyne prerequisites.
echo "[deadcode] cmd/gui is not buildable in this environment; running server-scope dead code check."
"${DEADCODE_BIN}" ./cmd/server ./internal/vpnserver
