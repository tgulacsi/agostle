#!/bin/sh
exec systemd-nspawn "--image=${SQFS:-./agostle.sqfs}" -a --suppress-sync=true --volatile=overlay "--bind=${AGOSTLE_BIN:-~/bin}:/app/bin" --chdir=/app /bin/bash
