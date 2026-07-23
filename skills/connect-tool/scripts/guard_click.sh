#!/usr/bin/env bash
# HOLD gate: refuse to click any element whose label matches an irreversible or
# outward-facing verb (post/publish/save/pay/delete/...) unless explicitly
# whitelisted for this step via ALLOW=<verb>. On a hold: screenshot + surface to
# the user, never click. Encodes "agree scope up front; hold the irreversible".
#
#   guard_click.sh <session> <target-selector-or-text>
#   ALLOW=authorize guard_click.sh <session> "button:has-text('Authorize')"
#   guard_click.sh --selfcheck
set -uo pipefail
OPENCLI=${OPENCLI:-opencli}
DENY='post|publish|tweet|send|save|pay|charge|subscribe|delete|remove|revoke|deploy|merge|confirm|transfer|authorize'

guard() {
  # Never /tmp: a HOLD screenshot can show a page mid-flow. Default to a 0700 run dir.
  local session=$1 target=$2 run_dir=${RUN_DIR:-$HOME/.config/connect-tool/runs/_holds} label
  label=$("$OPENCLI" browser "$session" find --selector "$target" 2>/dev/null \
            | grep -oiE "$DENY" | head -1 || true)
  label=$(printf '%s' "$label" | tr '[:upper:]' '[:lower:]')
  if [ -n "$label" ] && [ "$(printf '%s' "${ALLOW:-}" | tr '[:upper:]' '[:lower:]')" != "$label" ]; then
    local shot; mkdir -p "$run_dir"; chmod 700 "$run_dir" 2>/dev/null
    shot="$run_dir/HOLD-$(date -u +%Y%m%d-%H%M%S).png"
    "$OPENCLI" browser "$session" screenshot "$shot" >/dev/null 2>&1 || true
    echo "HOLD: '$target' matches irreversible verb '$label'. $shot saved. NOT clicking; surfaced for the user." >&2
    return 10
  fi
  "$OPENCLI" browser "$session" click "$target"
}

selfcheck() {
  local tmp; tmp=$(mktemp -d)
  cat > "$tmp/opencli" <<'STUB'
#!/usr/bin/env bash
# fake opencli: `find` returns a label with 'Save'; `click` prints; `screenshot` noop
case "$3" in
  find) echo '{"matches_n":1,"entries":[{"label":"Save changes"}]}';;
  click) echo "CLICKED $4";;
  screenshot) : ;;
esac
STUB
  chmod +x "$tmp/opencli"
  # irreversible verb with no ALLOW → HOLD (exit 10)
  if OPENCLI="$tmp/opencli" RUN_DIR="$tmp" guard sess "Save changes" 2>/dev/null; then
    echo "selfcheck FAIL: Save was not held"; exit 1; fi
  # same verb whitelisted → proceeds to click
  if ! OPENCLI="$tmp/opencli" RUN_DIR="$tmp" ALLOW=save guard sess "Save changes" 2>/dev/null | grep -q CLICKED; then
    echo "selfcheck FAIL: whitelisted verb did not click"; exit 1; fi
  rm -rf "$tmp"
  echo "guard_click.sh selfcheck OK"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi
guard "$@"
