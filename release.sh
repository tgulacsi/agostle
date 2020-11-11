#!/bin/sh
set -e
for nm in snapshot targets timestamp; do
	export "TUF_$(echo "$nm" | tr '[[:lower:]]' '[[:upper:]]')_PASSPHRASE=$(pass show U/UNOSOFT/TUF/${nm} | head -n1)"
done

set -x
fn=$(pwd)/tuf.pwd
cd "$(dirname "$0")"
DEST="${GOBIN:-"$(go env GOBIN)"}"
GOOS=linux GOARCH=amd64 go install &
GOOS=windows GOARCH=386 go build -o $DEST/agostle.exe &
(which tuf || go get -u github.com/theupdateframework/go-tuf/cmd/tuf)
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
for flavor in $FLAVORS; do
	EXT=
	if echo "$flavor" | grep -Fq windows; then
		EXT=.exe
	fi
	exe=$DEST/agostle${EXT}
	if [ -e "$DEST/$flavor" ]; then
		exe=$DEST/$flavor/agostle${EXT}
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
