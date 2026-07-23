#!/usr/bin/env bash
# Lane B: read ONE secret value rendered in the bound tab's DOM straight into the
# macOS Keychain, WITHOUT the value ever reaching model-visible stdout. The model
# authors the selector + destination; this script is the only process that touches
# the value, and it prints ONLY a redacted receipt (len / sha256[:8] / last4).
#
# Boundary: `opencli ... eval` output is captured into a shell var here and never
# echoed. set +x so a trace can't leak. Reads DOM innerText/value - NEVER pbpaste
# (OpenCLI's Copy button doesn't reach the pasteboard; pbpaste returns stale data).
# Fails loudly unless the selector matches EXACTLY ONE node.
#
#   grab_secret.sh --session S --selector CSS --service SVC --account ACCT [--attr value|textContent|auto]
#   grab_secret.sh --selfcheck
set -uo pipefail
set +x
OPENCLI=${OPENCLI:-opencli}
SECURITY=${SECURITY:-security}

grab() {
  local session=$1 selector=$2 service=$3 account=$4 attr=${5:-auto}
  local sel_b64 js raw line n b64 value len sha8 last4
  sel_b64=$(printf '%s' "$selector" | openssl base64 -A)
  js=$(cat <<JS
(() => {
  const sel = decodeURIComponent(escape(atob("$sel_b64")));
  const ns = document.querySelectorAll(sel);
  let out = "SECRET_ENVELOPE:" + ns.length + ":";
  if (ns.length === 1) {
    const el = ns[0];
    const raw = ("$attr" === "textContent") ? el.textContent
              : ("$attr" === "value")       ? el.value
              : (el.value !== undefined && el.value !== null && el.value !== "") ? el.value
              : (el.textContent || "");
    out += btoa(unescape(encodeURIComponent((raw || "").trim())));
  }
  return out;
})()
JS
)
  raw=$("$OPENCLI" browser "$session" eval "$js" 2>/dev/null)        # captured, NEVER echoed
  line=$(printf '%s\n' "$raw" | grep -oE 'SECRET_ENVELOPE:[0-9]+:[A-Za-z0-9+/=]*' | head -1)
  [ -n "$line" ] || { echo "FAIL: no envelope (selector/page problem)" >&2; return 3; }
  n=$(printf '%s' "$line" | cut -d: -f2)
  b64=$(printf '%s' "$line" | cut -d: -f3)
  case "$n" in
    1) : ;;
    0) echo "FAIL: selector matched 0 nodes" >&2; return 3;;
    *) echo "FAIL: selector matched $n nodes (need exactly 1)" >&2; return 3;;
  esac
  [ -n "$b64" ] || { echo "FAIL: empty value extracted" >&2; return 3; }
  value=$(printf '%s' "$b64" | openssl base64 -A -d 2>/dev/null)     # value lives ONLY here
  [ -n "$value" ] || { echo "FAIL: decode empty" >&2; return 3; }
  "$SECURITY" add-generic-password -U -a "$account" -s "$service" -w "$value" \
    || { echo "FAIL: keychain write" >&2; return 5; }
  len=${#value}
  sha8=$(printf '%s' "$value" | shasum -a 256 | cut -c1-8)
  last4=$(printf '%s' "$value" | tail -c 4)
  unset value b64 raw line
  echo "STORED service=$service account=$account len=$len sha256_8=$sha8 last4=$last4"
}

selfcheck() {
  command -v openssl >/dev/null || { echo "selfcheck SKIP: openssl missing"; exit 0; }
  local tmp secret; tmp=$(mktemp -d); secret='sk_test_ABC123'
  # fake opencli: eval returns a 1-node envelope carrying the secret
  cat > "$tmp/opencli" <<STUB
#!/usr/bin/env bash
# only implements: browser <s> eval <js>
B64=\$(printf '%s' '$secret' | openssl base64 -A)
echo "SECRET_ENVELOPE:1:\$B64"
STUB
  chmod +x "$tmp/opencli"
  # fake security: succeed WITHOUT echoing the value
  printf '#!/usr/bin/env bash\nexit 0\n' > "$tmp/security"; chmod +x "$tmp/security"
  local out
  out=$(OPENCLI="$tmp/opencli" SECURITY="$tmp/security" grab sess "input#k" SVC acct auto)
  grep -q "len=14" <<<"$out" || { echo "selfcheck FAIL: wrong len ($out)"; exit 1; }
  grep -q "last4=C123" <<<"$out" || { echo "selfcheck FAIL: wrong last4 ($out)"; exit 1; }
  if grep -q "$secret" <<<"$out"; then echo "selfcheck FAIL: SECRET LEAKED to stdout"; exit 1; fi
  # 0-node selector → fail loudly
  cat > "$tmp/opencli" <<'STUB'
#!/usr/bin/env bash
echo "SECRET_ENVELOPE:0:"
STUB
  chmod +x "$tmp/opencli"
  if OPENCLI="$tmp/opencli" SECURITY="$tmp/security" grab sess "input#k" SVC acct auto 2>/dev/null; then
    echo "selfcheck FAIL: 0-node selector did not fail"; exit 1; fi
  rm -rf "$tmp"
  echo "grab_secret.sh selfcheck OK (len/last4 right, no leak, 0-node fails)"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi

SESSION='' SEL='' SVC='' ACCT='' ATTR=auto
while [ $# -gt 0 ]; do
  case "$1" in
    --session) SESSION=$2; shift 2;;
    --selector) SEL=$2; shift 2;;
    --service) SVC=$2; shift 2;;
    --account) ACCT=$2; shift 2;;
    --attr) ATTR=$2; shift 2;;
    *) echo "unknown arg: $1" >&2; exit 2;;
  esac
done
[ -n "$SESSION" ] && [ -n "$SEL" ] && [ -n "$SVC" ] && [ -n "$ACCT" ] \
  || { echo "usage: grab_secret.sh --session S --selector CSS --service SVC --account ACCT [--attr ...]" >&2; exit 2; }
grab "$SESSION" "$SEL" "$SVC" "$ACCT" "$ATTR"
