#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
demo="$root/demo/keys"
trusted="$root/keys"
mkdir -p "$demo" "$trusted"

if [ -f "$demo/demo-signing-private.pem" ] && [ -f "$demo/demo-signing-public.pem" ]; then
  cp "$demo/demo-signing-public.pem" "$trusted/demo-2026.pem"
  chmod 0600 "$demo/demo-signing-private.pem"
  chmod 0644 "$demo/demo-signing-public.pem" "$trusted/demo-2026.pem"
  printf 'Reusing disposable demo keys under %s\n' "$demo"
  exit 0
fi

openssl genpkey -algorithm ED25519 -out "$demo/demo-signing-private.pem"
openssl pkey -in "$demo/demo-signing-private.pem" -pubout -out "$demo/demo-signing-public.pem"
cp "$demo/demo-signing-public.pem" "$trusted/demo-2026.pem"

chmod 0600 "$demo/demo-signing-private.pem"
chmod 0644 "$demo/demo-signing-public.pem" "$trusted/demo-2026.pem"
printf 'Generated disposable demo keys under %s\n' "$demo"
