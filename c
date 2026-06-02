#!/bin/sh
set -euo pipefail
CGO_ENABLED=0 go install
if [[ -e "${BRUNO_HOME:-}/bin/agostle" ]]; then
	(rsync -a "$(go env GOBIN)/agostle" "$BRUNO_HOME/bin/" && cd "$BRUNO_HOME/bin" && vm agostle)
fi
if command -v nix >/dev/null 2>/dev/null; then
	nix build .#dockerImage
	./result | docker load
fi

exec podman run -ti --rm -p 9500:9500 -v "$(go env GOBIN):/app/bin:ro" localhost/agostle "$@"
