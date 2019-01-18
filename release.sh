#!/bin/sh
set -e
set -x
fn=$(pwd)/tuf.pwd
cd "$(dirname "$0")"
GOOS=linux GOARCH=amd64 go install &
GOOS=windows GOARCH=386 go install &
(which tuf || go get -u github.com/flynn/go-tuf/cmd/tuf)
wait
FLAVORS="$*"
if [ -z "$FLAVORS" ]; then
	FLAVORS='linux_amd64 windows_386'
fi

if [ -e "$fn" ]; then
	. "$fn"
fi
TUF=${TUF:-$HOME/projects/TUF}

#rsync -avz --delete-before web:/var/www/www.unosoft.hu/agostle/ ./public/ || echo $?
mkdir -p "$TUF/staged/targets/agostle"
GOBIN="${GOBIN:-"$(go env GOBIN)"}"
for flavor in $FLAVORS; do
	EXT=
	if echo "$flavor" | grep -Fq windows; then
		EXT=.exe
	fi
	exe=$GOBIN/agostle${EXT}
	if [ -e "$GOBIN/$flavor" ]; then
		exe=$GOBIN/$flavor/agostle${EXT}
	fi
	rsync -avz "${exe}" "$TUF/staged/targets/agostle/${flavor}"
	(cd "$TUF" && tuf -d "$TUF" add "agostle/$flavor")
done
(cd "$TUF" &&
tuf snapshot &&
tuf timestamp &&
tuf commit
)

rsync -avz --delete-after "$TUF/repository/" web:/var/www/www.unosoft.hu/tuf/
