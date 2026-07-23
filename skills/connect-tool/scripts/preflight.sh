#!/usr/bin/env bash
# Pin OpenCLI as THE browser driver, bind the user's already-open focused Chrome
# tab, and PROVE we're on the real bound tab. NEVER falls through to a spawned
# browser (Playwright, Puppeteer, headless Chromium): those carry no login.
# If OpenCLI is missing/disconnected, prints the bootstrap path (offer to install).
#
#   preflight.sh --deps             # one-shot setup check: every dependency
#   preflight.sh [session]          # default session: connecttool
#   preflight.sh --selfcheck
set -uo pipefail
OPENCLI=${OPENCLI:-opencli}

bootstrap_hint() {
  cat >&2 <<'EOF'
--- OpenCLI bridge not ready. To set it up (see references/opencli-bootstrap.md): ---
  1. Install CLI:        npm install -g @jackwener/opencli
  2. Install extension:  https://github.com/jackwener/opencli/releases
  3. Open Chrome, focus the tab you want, then re-run.
  Verify any time with:  opencli doctor   (want [OK] Daemon + [OK] Extension)
NOTE: this skill drives your REAL Chrome via OpenCLI bind. It will NOT spawn a browser.
EOF
}

# One-shot dependency check. Run this once at setup (and any time something is
# off); it reports EVERY missing piece instead of failing on the first one.
deps() {
  local missing=0
  check() {  # $1=label $2=command $3=how-to-install
    if command -v "$2" >/dev/null 2>&1; then
      printf '  OK   %-9s (%s)\n' "$1" "$(command -v "$2")"
    else
      printf '  MISS %-9s -> %s\n' "$1" "$3"; missing=1
    fi
  }
  echo "connect-tool dependency check"
  if [ "$(uname -s)" = "Darwin" ]; then
    printf '  OK   %-9s (%s)\n' macOS "$(sw_vers -productVersion 2>/dev/null || echo darwin)"
  else
    printf '  MISS %-9s -> connect-tool is macOS-only (it stores secrets in the macOS Keychain)\n' macOS
    missing=1
  fi
  check security  security  "macOS built-in; missing means this is not macOS"
  if [ -d "${CHROME_APP:-/Applications/Google Chrome.app}" ]; then
    printf '  OK   %-9s (%s)\n' chrome "${CHROME_APP:-/Applications/Google Chrome.app}"
  else
    printf '  MISS %-9s -> install Google Chrome from https://www.google.com/chrome/\n' chrome; missing=1
  fi
  check node      node      "brew install node   (needed to install opencli)"
  check npm       npm       "ships with node; reinstall node if missing"
  check opencli   "$OPENCLI" "npm install -g @jackwener/opencli"
  # Either uv or a new-enough python3 runs the helpers; only one is required.
  if command -v uv >/dev/null 2>&1; then
    printf '  OK   %-9s (%s)\n' python "uv: $(command -v uv)"
  elif command -v python3 >/dev/null 2>&1 \
       && python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3,12) else 1)' 2>/dev/null; then
    printf '  OK   %-9s (%s, no uv needed)\n' python "$(python3 -V 2>&1)"
  else
    printf '  MISS %-9s -> brew install uv   (or install Python 3.12+)\n' python; missing=1
  fi
  check jq        jq        "brew install jq     (verify_use.sh asserts on JSON receipts)"
  check openssl   openssl   "macOS built-in"
  check shasum    shasum    "macOS built-in"
  if command -v "$OPENCLI" >/dev/null 2>&1; then
    local doc; doc=$("$OPENCLI" doctor 2>&1 || true)
    if grep -qE '\[OK\] Daemon' <<<"$doc"; then echo "  OK   daemon"
    else echo "  MISS daemon    -> opencli daemon restart"; missing=1; fi
    if grep -qE '\[OK\] Extension' <<<"$doc"; then echo "  OK   extension"
    else echo "  MISS extension -> load the OpenCLI Chrome extension, then hit reload on its card in chrome://extensions/"; missing=1; fi
  fi
  if [ "$missing" -eq 0 ]; then
    echo "All dependencies present. Focus the Chrome tab you want and run: preflight.sh <target-slug>"
  else
    echo "Install the MISS items above, then re-run: preflight.sh --deps" >&2
  fi
  return "$missing"
}

preflight() {
  local session=${1:-connecttool} doc state url
  if ! command -v "$OPENCLI" >/dev/null 2>&1; then
    echo "FAIL: opencli not installed." >&2; bootstrap_hint; return 3
  fi
  doc=$("$OPENCLI" doctor 2>&1 || true)
  if ! grep -qE '\[OK\] Daemon' <<<"$doc"; then
    echo "FAIL: OpenCLI daemon not running." >&2; bootstrap_hint; return 4
  fi
  if ! grep -qE '\[OK\] Extension' <<<"$doc"; then
    echo "FAIL: OpenCLI Chrome extension not connected." >&2; bootstrap_hint; return 4
  fi
  "$OPENCLI" browser "$session" bind >/dev/null 2>&1 \
    || { echo "FAIL: bind failed - focus the Chrome tab you want, then retry." >&2; return 5; }
  state=$("$OPENCLI" browser "$session" state 2>/dev/null || true)
  url=$(grep -oiE 'https?://[^ "]+' <<<"$state" | head -1)
  if [ -z "$url" ] || grep -qi 'about:blank' <<<"$state"; then
    echo "FAIL: bound tab has no real URL (got: ${url:-none}). Focus a real page." >&2; return 6
  fi
  # Print scheme+host+path only. An OAuth callback URL carries ?code= in its query,
  # and that belongs to the consuming CLI, never to this output.
  echo "OK bound session=$session url=${url%%[?#]*}"
}

make_stub() {  # $1=dir $2=extension-status(connected|missing)
  cat > "$1/opencli" <<STUB
#!/usr/bin/env bash
case "\$1" in
  doctor) echo "[OK] Daemon: running on port 19825"; \
          [ "$2" = "connected" ] && echo "[OK] Extension: connected (v1.0.19)" || echo "[--] Extension: not connected";;
  browser)
    case "\$3" in
      bind) exit 0;;
      state) echo '{"url":"https://example.com/settings/api-keys","title":"API Keys"}';;
    esac;;
esac
STUB
  chmod +x "$1/opencli"
}

selfcheck() {
  local tmp; tmp=$(mktemp -d)
  make_stub "$tmp" connected
  OPENCLI="$tmp/opencli" preflight sess | grep -q '^OK bound' \
    || { echo "selfcheck FAIL: healthy bridge not accepted"; exit 1; }
  make_stub "$tmp" missing
  if OPENCLI="$tmp/opencli" preflight sess 2>/dev/null; then
    echo "selfcheck FAIL: disconnected extension accepted"; exit 1; fi
  # missing binary path → bootstrap (exit 3)
  if OPENCLI="$tmp/does-not-exist" preflight sess 2>/dev/null; then
    echo "selfcheck FAIL: missing opencli accepted"; exit 1; fi
  # --deps must report every miss, not die on the first one (setup UX).
  # Capture first: `grep -q` exits on match, and pipefail would read the
  # resulting SIGPIPE on deps as a failure.
  make_stub "$tmp" connected
  local out; out=$(OPENCLI="$tmp/does-not-exist" deps 2>/dev/null)
  grep -q 'MISS opencli' <<<"$out" \
    || { echo "selfcheck FAIL: --deps did not flag a missing opencli"; exit 1; }
  grep -qE '(OK|MISS) +jq' <<<"$out" \
    || { echo "selfcheck FAIL: --deps stopped before checking jq"; exit 1; }
  rm -rf "$tmp"
  echo "preflight.sh selfcheck OK"
}

case "${1:-}" in
  --selfcheck) selfcheck; exit 0;;
  --deps) deps; exit $?;;
esac
preflight "$@"
