#!/bin/sh
yum install python python-virtualenv
virtualenv --system-site-packages venv
. ./venv/bin/activate
pip install -r apostle/requirements.txt
