#!/bin/sh
# Builds crontab-dashboard for one platform and writes the release archive
# with .sha256 and .md5 files, named like the cronitor assets:
#   crontab-dashboard_<os>_<arch>.tar.gz (Windows: .zip)
# The binary is the archive's only file, which `cronitor update` expects.
# Run `npm run build` in web/ first.
#
#   scripts/package-dashboard.sh linux amd64 dist
set -eu
goos=${1:?usage: package-dashboard.sh GOOS GOARCH OUTDIR}
goarch=${2:?usage: package-dashboard.sh GOOS GOARCH OUTDIR}
out=${3:?usage: package-dashboard.sh GOOS GOARCH OUTDIR}
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

test -s "$root/web/static/index.html" || { echo "web/static/index.html is missing; run npm run build in web/" >&2; exit 1; }

name=crontab-dashboard
binary=$name
[ "$goos" = windows ] && binary=$name.exe
asset=${name}_${goos}_${goarch}
mkdir -p "$out"
out=$(CDPATH= cd -- "$out" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

(cd "$root" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "-s -w" -o "$stage/$binary" ./cmd/crontab-dashboard)

if [ "$goos" = windows ]; then
	archive=$asset.zip
	(cd "$stage" && zip -q "$out/$archive" "$binary")
else
	archive=$asset.tar.gz
	tar -czf "$out/$archive" -C "$stage" "$binary"
fi
cd "$out"
sha256sum "$archive" | cut -d ' ' -f 1 > "$archive.sha256"
md5sum "$archive" | cut -d ' ' -f 1 > "$archive.md5"
echo "$out/$archive"
