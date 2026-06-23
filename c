#!/bin/sh
set -euo pipefail
CGO_ENABLED=0 go install
for dest in "${BRUNO_HOME:-}/bin/" /home/alfas/bra3/dev/bruno/bin/; do
	if [[ ! -e "${dest}" ]]; then
		continue
	fi
	if [[ -d "${dest}/agostle" ]]; then
		dest="${dest}/agostle/"
	fi
	if test -L "${dest}/agostle"; then
		rm "${dest}/agostle"
	fi
	(rsync -a "$(go env GOBIN)/agostle" "$dest/agostle" && cd "$dest" && vm agostle) || echo $?
done
if command -v nix >/dev/null 2>/dev/null; then
	nix build .#dockerImage
	./result | docker load
fi

if ! br_ctr restart agostle; then
	exec podman run -ti --rm -p 9500:9500 -v "$(go env GOBIN):/app/bin:ro" localhost/agostle "$@"
fi
