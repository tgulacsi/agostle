#!/bin/sh
set -e
wget http://ftp.bme.hu/pub/mirrors/tdf/libreoffice/stable/4.1.1/rpm/x86_64/LibreOffice_4.1.1_Linux_x86-64_rpm.tar.gz
tar xaf LibreOffice_4.1.1_Linux_x86-64_rpm.tar.gz
cd LibreOffice_1.4*/
yum localinstall RMPS/*
cd /usr/bin
ln -s libreoffice4* loffice
