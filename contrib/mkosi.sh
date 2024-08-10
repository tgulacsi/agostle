#!/bin/sh
set -eu
name="${1:-agostle}"
CACHEDIR="${CACHEDIR:-/tmp/agostle-debootstrap}"
mkdir -p "${CACHEDIR}"

# get the needed packages
#   pdftk is not available
#   libreoffice is not installable by debootstrap
pwd="$(cd "$(dirname "$0")"; pwd)"
packages="$(sed -ne '/ install / { s/^.* install //; s/--[^ ]*//g; s/#.*$//; s/^  *//; p; }' "${pwd}/../docks/debian/Dockerfile" | tr ' ' '\n' | sort -u | grep -vE 'ttf-mscorefonts|pdftk' | tr '\n' , | sed -e 's/,,,*/,/g; s/^,//; s/,$//')"
echo "# packages=$packages" >&2

tempdir="$(mktemp -d --tmpdir tmp.agostle-mkosi.XXX)"
trap 'rm -rf "$tempdir"' EXIT 
(
cd "$tempdir"

# if [ "${SKIP_DEBOOTSTRAP:-0}" -ne 1 ]; then
printf '#!/bin/sh -eux\napt -y install ttf-mscorefonts-installer' >mkosi.postinst
printf '#!/bin/sh -u\napt -y clean\napt -y distclean' >mkosi.finalize
chmod +x mkosi.[pf]*
mkdir -p mkosi.extra/etc
cp -aL /etc/resolv.conf mkosi.extra/etc/

set -x
time mkosi --distribution=debian "--release=${SUITE:-testing}" \
  --repositories=main,contrib,non-free,non-free-firmware \
  "--package=$packages,systemd-container" \
  "--package-cache-dir=${CACHEDIR}" \
  --extra-tree=mkosi.extra \
  --bootable=no \
  --format=tar \
  --with-network=yes \
  --output="${name}"

set +x
)

# fi
# create the root sqfs
# tar cf - . -C "${name}" | tar2sqfs -c zstd -f "${sqfs}"
zstd -dc "${tempdir}/${name}".tar.zst | tar2sqfs -c zstd -f "${name}.sqfs"

rm -rf "${tempdir}"
