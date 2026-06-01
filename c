#!/bin/sh
set -euo pipefail
CGO_ENABLED=0 go install
if command -v nix >/dev/null 2>/dev/null; then
	env NIX_ALLOW_UNFREE=1 nix build .#dockerImage --impure
	./result | docker load
fi

exec podman run -ti --rm -p 9500:9500 -v "$(go env GOBIN):/app/bin:ro" localhost/agostle "$@"
