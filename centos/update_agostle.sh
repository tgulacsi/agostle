#!/bin/sh
set -e
go get -u github.com/tgulacsi/agostle
ver=$(cd ~/src/github.com/tgulacsi/agostle/ \
	&& git log --oneline | head -n1 | cut -d' ' -f1)
[ -n "$ver" ]
ver=$(date '+%Y%m%d')-$ver
echo ver=$ver
echo "Copying to 192.168.3.110"
if scp -p bin/agostle 192.168.3.110:prd/agostle-$ver; then
#ssh 192.168.3.110 sh -c 'cd /home/agostle && sudo -u agostle ln -sf agostle-${ver} agostle && killall agostle'
#ssh -t 192.168.3.110 sh -c "cd /home/tgulacsi/prd && ln -sf agostle-${ver} $HOME/prd/agostle && sudo -u agostle killall agostle && sudo -u agostle find /var/tmp/agostle/ -delete"
	ssh -t 192.168.3.110 sh -c "cd /home/tgulacsi/prd && ln -sf agostle-${ver} $HOME/prd/agostle && sudo -u agostle killall agostle"
fi

GOOS=windows GOARCH=386 go get -u github.com/tgulacsi/agostle

echo "copying agostle.exe to 192.168.1.2:html/"
set +e
echo rsync -av bin/agostle.exe 192.168.1.2:html/
rsync -av bin/windows_386/agostle.exe 192.168.1.2:html/

