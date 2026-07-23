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
  # These land inside a generated executable, so they are validated, not trusted.
  case "$name" in *[!A-Za-z0-9._-]*|""|.*) echo "FAIL: bad wrapper name '$name'" >&2; return 2;; esac
  case "$envvar" in [!A-Za-z_]*|*[!A-Za-z0-9_]*|"") echo "FAIL: bad env var '$envvar'" >&2; return 2;; esac
  case "$acct$svc" in *[!A-Za-z0-9._@-]*|"") echo "FAIL: bad keychain account/service" >&2; return 2;; esac
  case "$real" in /*) : ;; *) echo "FAIL: real binary must be an absolute path" >&2; return 2;; esac
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
  for bad in "../evil" "a b" ""; do
    if HOME="$tmp" mint "$bad" X a s /usr/bin/true >/dev/null 2>&1; then
      echo "selfcheck FAIL: bad wrapper name '$bad' accepted"; exit 1; fi
  done
  if HOME="$tmp" mint ok "BAD VAR" a s /usr/bin/true >/dev/null 2>&1; then
    echo "selfcheck FAIL: bad env var accepted"; exit 1; fi
  if HOME="$tmp" mint ok OK a s relative/path >/dev/null 2>&1; then
    echo "selfcheck FAIL: relative binary path accepted"; exit 1; fi
  rm -rf "$tmp"
  echo "mint_wrapper.sh selfcheck OK (keychain read + arg validation)"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi
[ $# -eq 5 ] || { echo "usage: mint_wrapper.sh <name> <ENV_VAR> <kc-account> <kc-service> <real-binary>" >&2; exit 2; }
mint "$@"
