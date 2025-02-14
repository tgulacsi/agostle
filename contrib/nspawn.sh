#!/bin/sh
set -eu
echo AGOSTLE_BIN=${AGOSTLE_BIN:=~/bin}
set -x
exec sudo systemd-nspawn "--image=${SQFS:-./agostle.sqfs}" -a --suppress-sync=true --volatile=overlay "--bind=${AGOSTLE_BIN}:/app/bin" --chdir=/app /app/bin/agostle "$@"
