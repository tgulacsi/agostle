#!/bin/sh
${AGOSTLE_BIN:=~/bin}
exec sudo systemd-nspawn "--image=${SQFS:-./agostle.sqfs}" -a --suppress-sync=true --volatile=overlay "--bind=${AGOSTLE_BIN}:/app/bin" --chdir=/app /app/bin/agostle "$@"
