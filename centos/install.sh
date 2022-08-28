#!/bin/sh
set -e
sudo useradd -l agostle
yum install libreoffice GraphicsMagick ghostscript
if ! yum install pdftk-1.44-2.el6.rf.x86_64.rpm runit-2.1.1-6.el6.x86_64.rpm; then
	. "$(dirname "$0")/install_pdftk.sh"
	. "$(dirname "$0")/install_runit.sh"
fi
cp -pr "$(dirname "$0")/etc/service/agostle" /etc/service/
