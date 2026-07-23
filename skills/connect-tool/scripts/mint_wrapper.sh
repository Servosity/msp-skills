#!/usr/bin/env bash
# Stamp a ~/.local/bin/<cli> Keychain wrapper so the consuming CLI reads its
# secret from the Keychain at launch - the value never lives in a config file or
# env export the model can see.
#
#   mint_wrapper.sh <wrapper-name> <ENV_VAR> <kc-account> <kc-service> <real-binary-path>
#   e.g. mint_wrapper.sh halopsa-cli HALOPSA_API_KEY halopsa HALOPSA_API_KEY "$HOME/go/bin/halopsa-cli"
#   mint_wrapper.sh --selfcheck
set -uo pipefail

mint() {
  local name=$1 envvar=$2 acct=$3 svc=$4 real=$5
  local bindir="$HOME/.local/bin" dest
  mkdir -p "$bindir"
  dest="$bindir/$name"
  cat > "$dest" <<EOF
#!/usr/bin/env bash
# connect-tool Keychain wrapper for $name. Secret stays in Keychain; never in a file.
if [ -z "\${$envvar:-}" ]; then
  $envvar="\$(security find-generic-password -a $acct -s $svc -w 2>/dev/null)"
  export $envvar
fi
exec "$real" "\$@"
EOF
  chmod +x "$dest"
  echo "wrote wrapper $dest ($envvar <- keychain $acct/$svc -> $real)"
}

selfcheck() {
  local tmp; tmp=$(mktemp -d)
  HOME="$tmp" mint halopsa-cli HALOPSA_API_KEY halopsa HALOPSA_API_KEY /usr/bin/true >/dev/null
  local w="$tmp/.local/bin/halopsa-cli"
  test -x "$w" || { echo "selfcheck FAIL: wrapper not executable"; exit 1; }
  grep -q 'security find-generic-password -a halopsa -s HALOPSA_API_KEY -w' "$w" \
    || { echo "selfcheck FAIL: keychain read line missing"; exit 1; }
  grep -q 'exec "/usr/bin/true"' "$w" || { echo "selfcheck FAIL: exec line missing"; exit 1; }
  rm -rf "$tmp"
  echo "mint_wrapper.sh selfcheck OK"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi
[ $# -eq 5 ] || { echo "usage: mint_wrapper.sh <name> <ENV_VAR> <kc-account> <kc-service> <real-binary>" >&2; exit 2; }
mint "$@"
