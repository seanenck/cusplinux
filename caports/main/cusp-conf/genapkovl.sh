#!/bin/sh -e

HOSTNAME=""
OUT=""
while getopts "h:o:" opt ; do
  case $opt in
    h) HOSTNAME="$OPTARG";;
    o) OUT="$OPTARG";;
    *)
      _usage
      ;;
  esac
done

[ -z "$HOSTNAME" ] && echo "no hostname set" && exit 1
[ -z "$OUT" ] && echo "no output directory set" && exit 1
mkdir -p "$OUT"

cleanup() {
	rm -rf "$tmp"
}

rc_add() {
	mkdir -p "$tmp"/etc/runlevels/"$2"
	ln -sf /etc/init.d/"$1" "$tmp"/etc/runlevels/"$2"/"$1"
}

tmp="$(mktemp -d)"
trap cleanup EXIT

[ ! -d etc/ ] && echo "no etc directory" && exit 1

mkdir -p "$tmp"/etc
echo "$HOSTNAME" > "$tmp/etc/hostname"
rsync -ac etc/ "$tmp/etc"
if [ -d services ]; then
  for dir in services/*; do
    for file in "$dir/"*; do
      level=$(basename "$dir")
      svc=$(basename "$file")
      rc_add "$svc" "$level"
    done
  done
fi
if [ -d scripts ]; then
  for script in scripts/*; do
    [ -x "$script" ] && "$script" "$tmp"
  done
fi

APKOVL="$HOSTNAME.apkovl.tar.gz"
tar -c --owner 0 --group 0 -C "$tmp" etc | gzip -9n > "$OUT/$APKOVL"
(cd "$OUT" && mksquashfs "$APKOVL" apkovl.sqfs)
