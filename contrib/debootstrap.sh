#!/bin/sh
set -eu
dest="${1:-./agostle-root}"
sqfs="${2:-./agostle.sqfs}"
CACHEDIR="${CACHEDIR:-/tmp/agostle-debootstrap}"

# get the needed packages
#   pdftk is not available
#   libreoffice is not installable by debootstrap
pwd="$(cd "$(dirname "$0")"; pwd)"
packages="$(sed -ne '/ install / { s/^.* install //; s/--[^ ]*//g; s/#.*$//; s/^  *//; p; }' "${pwd}/../docks/debian/Dockerfile" | tr ' ' '\n' | sort -u | grep -vE 'libreoffice|pdftk' | tr '\n' , | sed -e 's/,,,*/,/g; s/^,//; s/,$//')"
echo "# packages=$packages" >&2

if [ "${SKIP_DEBOOTSTRAP:-0}" -ne 1 ]; then
mkdir -p "${CACHEDIR}"
sudo rm -rf "${dest}"
set +e
set -x
time sudo /usr/sbin/debootstrap --merged-usr \
  --include="$packages" \
  --components=main,contrib,non-free,non-free-firmware \
  "--cache-dir=${CACHEDIR}" \
  "${SUITE:-testing}" "${dest}" http://httpredir.debian.org/debian
set +x
set -e
fi

# fix the installation and install libreoffice
sudo tee "${dest}/fix.sh" <<EOF
#!/bin/sh
set -x
apt-get -y update
apt -y --fix-broken install
apt -y dist-upgrade
apt -y install libreoffice
apt -y clean
apt -y distclean
EOF

sudo sh <<EOF
umount -f "${dest}/proc"
umount -f "${dest}/sys"

mount proc "${dest}"/proc -t proc
mount sysfs "${dest}"/sys -t sysfs
#mount -o bind "${CACHEDIR}" "${dest}/var/cache/apt/archives"
chmod 0755 "${dest}/fix.sh"

chroot "${dest}" ./fix.sh

#umount "${dest}/var/cache/apt/archives"
umount "${dest}"/proc
umount "${dest}"/sys
EOF

sudo rm "${dest}/fix.sh" 

# create the root sqfs
tar cf - -C "${dest}" . | tar2sqfs -c zstd -f "${sqfs}"
sudo rm -rf "${dest}"
