#!/bin/sh
set -eu
if ! [ -e opt/libreoffice*/share/fonts/truetype ]; then
  if ! [ -e LibreOffice-fresh.full-x86_64.AppImage ]; then
    wget https://appimages.libreitalia.org/LibreOffice-fresh.full-x86_64.AppImage
  fi
  ./LibreOffice-fresh.full-x86_64.AppImage --appimage-extract
  cd squashfs-root
fi

if [ -e ../LibreOffice.x86_64.AppImage ]; then
  echo "../LibreOffice.x86_64.AppImage already exists" >&2
  exit 0
fi


if ! [ -e opt/libreoffice*/share/fonts/truetype/times.ttf ]; then
  sudo apt install ttf-mscorefonts-installer
  rsync -a /usr/share/fonts/truetype/msttcorefonts/ opt/libreoffice*/share/fonts/truetype/
fi

if ! [ -e opt/libreoffice*/share/fonts/truetype/TakaoMincho.ttf ]; then
  for pkg in fonts-sil-gentium fonts-dejavu-extra fonts-takao-mincho; do
    apt download "$pkg"
    ls "$pkg"*.deb
    ar p "$pkg"*.deb data.tar.xz | tar xf - -J --wildcards '*.ttf' 
    find usr/share/fonts/ -type f -name '*.ttf' -print0 | xargs -0 -r -i mv {} opt/libreoffice*/share/fonts/truetype/
    rm "$pkg"*.deb
  done
fi

if ! command -v appimagetool; then
  wget 'https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage'
  rsync -a appimagetool-x86_64.AppImage /usr/local/bin/appimagetool --chmod a+rx
fi

appimagetool . ../LibreOffice.x86_64.AppImage
