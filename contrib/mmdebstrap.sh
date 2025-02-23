#!/bin/sh
set -eu
dest="${1:-/var/lib/machines/agostle}"
sqfs="${2:-./agostle.sqfs}"

# get the needed packages
#   pdftk is not available
#   libreoffice is not installable by mmdebstrap
pwd="$(cd "$(dirname "$0")"; pwd)"
packages="$(sed -ne '/ install / { s/^.* install //; s/--[^ ]*//g; s/#.*$//; s/^  *//; p; }' "${pwd}/../docks/debian/Dockerfile" | tr ' ' '\n' | sort -u | grep -vE 'libreOffice|pdftk' | tr '\n' , | sed -e 's/,,,*/,/g; s/^,//; s/,$//')"
echo "# packages=$packages" >&2

if [ "${SKIP_MMDEBSTRAP:-0}" -ne 1 ]; then
sudo rm -rf "${dest}"
set +e
if ! command -v mmdebstrap; then
  sudo apt install mmdebstrap
fi
set -x
sudo btrfs subv delete "${dest}"
sudo btrfs subv create "${dest}"
time sudo /usr/bin/mmdebstrap \
  --include="$packages",systemd-container,auto-apt-proxy \
  --variant=required \
  --components=main,contrib,non-free \
  --aptopt='APT::Solver "aspcud"' \
  "${SUITE:-testing}" \
  "${dest}"
sudo rm "${dest}/etc/hostname"
set +x
set -e
fi


# create the root sqfs
tar cf - -C "${dest}" --exclude=etc/hostname . | tar2sqfs -c zstd -f "${sqfs}"

echo 'if ! [ -d /var/lib/machines/agostle ]; then sudo btrfs subv create /var/lib/machines/agostle; fi; rdsquashfs -u / -p /var/lib/machines/agostle ./agostle.sqfs '
# echo sudo mkdir -p /var/lib/machines/agostle; sudo mount -t squashfs ./agostle.sqfs /var/lib/machines/agostle
#
echo 'sudo systemd-nspawn -D /var/lib/machines/agostle --volatile -a --tmpfs /app --bind-ro ~/bin:/app/bin --chdir=/app -U  --link-journal=try-guest -P ./bin/agostle serve :9500'
