#!/bin/sh
set -e
yum install rpm-build rpmdevtools
wget -O runit-rpm.zip https://github.com/imeyer/runit-rpm/archive/master.zip
unzip runit-rpm.zip
cd runit-rpm-master
sudo yum install glibc-static
./build.sh
sudo yum install $HOME/rpmbuild/RPMS/x86_64/runit-*.rpm
