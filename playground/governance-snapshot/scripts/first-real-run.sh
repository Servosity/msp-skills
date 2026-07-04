#!/usr/bin/env bash
#
# first-real-run.sh - generate the FIRST real third-party-app-consent governance
# report against Servosity's own Microsoft 365 tenant, and PROVE it is not canned.
#
# This script is ARMED, not faked. It does real work only once a real
# MICROSOFT_GRAPH_TOKEN exists (from the connect-tool auth - see
# ../NEEDS-DAMIEN-auth.md). Until then it dry-runs cleanly to the auth boundary
# and stops. It never fabricates a live report.
#
# Usage:
#   ./first-real-run.sh            # run for real if a token is present; else stop at the auth boundary
#   ./first-real-run.sh --dry-run  # always stop at the auth boundary (prove the path without auth)
#
# Token resolution order:
#   1. $MICROSOFT_GRAPH_TOKEN in the environment
#   2. macOS Keychain: service $KEYCHAIN_SERVICE (default MICROSOFT_GRAPH_TOKEN),
#      account $KEYCHAIN_ACCOUNT (default servosity)
#
set -euo pipefail

DRY_RUN=0
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SNAP_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$SNAP_DIR/../.." && pwd)"
CLI_SRC="$REPO_ROOT/skills/microsoft-graph/cli"
REPORT_PY="$SNAP_DIR/report.py"
DEMO_SAMPLE="$SNAP_DIR/samples/demo-consent-audit.json"
OUT_DIR="$SNAP_DIR/out"
KEYCHAIN_SERVICE="${KEYCHAIN_SERVICE:-MICROSOFT_GRAPH_TOKEN}"
KEYCHAIN_ACCOUNT="${KEYCHAIN_ACCOUNT:-servosity}"

say()  { printf '%s\n' "$*"; }
step() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die()  { printf '\033[31merror: %s\033[0m\n' "$*" >&2; exit 1; }

boundary() {
  # Print the auth-boundary message and stop cleanly (exit 0). Reaching here is
  # the DESIGNED end of a dry run, not a failure.
  step "AUTH BOUNDARY - no live report generated (by design)"
  cat <<EOF
No MICROSOFT_GRAPH_TOKEN is available yet, so nothing was run against a live tenant.

To arm this script, complete the one-time read-only auth:
  1. Follow the 10-minute connect-tool steps in:
       $SNAP_DIR/NEEDS-DAMIEN-auth.md
  2. That stores a read-only Graph token in the macOS Keychain
     (service "$KEYCHAIN_SERVICE", account "$KEYCHAIN_ACCOUNT").
  3. Re-run this exact command:
       $SCRIPT_DIR/first-real-run.sh

Preflight status (everything below the token is already verified and ready):
  CLI source:     $( [[ -d "$CLI_SRC" ]] && echo "found ($CLI_SRC)" || echo "MISSING" )
  report engine:  $( [[ -f "$REPORT_PY" ]] && echo "found" || echo "MISSING" )
  demo sample:    $( [[ -f "$DEMO_SAMPLE" ]] && echo "found" || echo "MISSING" )
  CLI binary:     ${CLI_BIN:-<will build on first real run>}

The live report does not exist until the auth above happens.
EOF
  exit 0
}

step "Preflight (no tenant access needed)"
[[ -d "$CLI_SRC" ]]    || die "CLI source not found at $CLI_SRC"
[[ -f "$REPORT_PY" ]]  || die "report engine not found at $REPORT_PY"
[[ -f "$DEMO_SAMPLE" ]] || die "demo sample not found at $DEMO_SAMPLE (needed for the anti-canned diff)"
command -v python3 >/dev/null || die "python3 required"
say "  CLI source, report engine, and demo sample all present."

# Resolve (or build) the CLI binary so the boundary message can report it, but
# do NOT require a token to get this far.
CLI_BIN="$(command -v microsoft-graph-cli || true)"
if [[ -z "$CLI_BIN" ]]; then
  say "  microsoft-graph-cli not on PATH; will build from source when needed."
fi

# --- Token resolution -------------------------------------------------------
TOKEN="${MICROSOFT_GRAPH_TOKEN:-}"
TOKEN_SOURCE="env:MICROSOFT_GRAPH_TOKEN"
if [[ -z "$TOKEN" ]] && command -v security >/dev/null; then
  if TOKEN="$(security find-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w 2>/dev/null)"; then
    TOKEN_SOURCE="keychain:$KEYCHAIN_SERVICE/$KEYCHAIN_ACCOUNT"
  else
    TOKEN=""
  fi
fi

if [[ "$DRY_RUN" == "1" ]]; then
  say "  --dry-run: stopping at the auth boundary regardless of token presence."
  boundary
fi
[[ -n "$TOKEN" ]] || boundary

# --- ARMED: a real token is present. Everything below hits the live tenant. --
export MICROSOFT_GRAPH_TOKEN="$TOKEN"
say "  Token resolved from $TOKEN_SOURCE."

if [[ -z "$CLI_BIN" ]]; then
  step "Building microsoft-graph-cli"
  CLI_BIN="$OUT_DIR/microsoft-graph-cli"
  mkdir -p "$OUT_DIR"
  ( cd "$CLI_SRC" && go build -o "$CLI_BIN" ./cmd/microsoft-graph-cli )
  say "  built $CLI_BIN"
fi

mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
RAW_JSON="$OUT_DIR/real-consent-audit.json"
DOCTOR_TXT="$OUT_DIR/doctor.txt"
REPORT_MD="$OUT_DIR/servosity-report-$STAMP.md"
RECEIPTS_MD="$OUT_DIR/receipts-$STAMP.md"

step "Verifying auth against the live tenant (doctor)"
"$CLI_BIN" doctor > "$DOCTOR_TXT" 2>&1 || {
  say "  doctor reported a problem:"; sed 's/^/    /' "$DOCTOR_TXT"
  die "auth check failed - the token may be expired or missing scopes (need Application.Read.All, Directory.Read.All, DelegatedPermissionGrant.Read.All)."
}
say "  auth OK."

step "Running the read-only consent audit against the live tenant"
"$CLI_BIN" apps consent --json > "$RAW_JSON" || die "apps consent failed (see stderr above)."
say "  wrote raw audit -> $RAW_JSON"

step "Proving the output is NOT the canned demo sample"
python3 - "$RAW_JSON" "$DEMO_SAMPLE" <<'PY'
import json, sys, hashlib
real_p, demo_p = sys.argv[1], sys.argv[2]
real = json.load(open(real_p)); demo = json.load(open(demo_p))
def names(d): return sorted(a.get("displayName","") for a in d.get("apps", []))
def h(p):  return hashlib.sha256(open(p,'rb').read()).hexdigest()[:16]
if h(real_p) == h(demo_p):
    sys.exit("REFUSING: live output is byte-identical to the synthetic demo sample - that cannot be real.")
same_summary = real.get("summary") == demo.get("summary")
same_names = names(real) == names(demo)
if same_summary and same_names:
    sys.exit("REFUSING: live output has the same summary AND the same app names as the synthetic demo - refusing to present canned data.")
print(f"  PROOF: live sha256[:16]={h(real_p)}  demo sha256[:16]={h(demo_p)}  (differ)")
print(f"  live third-party apps: {real['summary'].get('thirdPartyApps')}  vs demo: {demo['summary'].get('thirdPartyApps')}")
print(f"  live app names differ from demo: {names(real) != names(demo)}")
print("  -> this report is computed from Servosity's real tenant, not the sample.")
PY

step "Rendering the branded report"
python3 "$REPORT_PY" "$RAW_JSON" --org "Servosity" -o "$REPORT_MD"
say "  wrote report -> $REPORT_MD"

step "Writing receipts (spot-check these against the Entra admin portal)"
{
  echo "# first-real-run receipts - $STAMP"
  echo
  echo "Generated by \`$(basename "$0")\` from Servosity's live Microsoft 365 tenant."
  echo "Token source: $TOKEN_SOURCE (value never printed)."
  echo
  echo "## Headline counts (verify in Entra admin center)"
  echo
  echo "Entra admin center → Identity → Applications → Enterprise applications → All applications,"
  echo "then per app → Security → Permissions. The counts below must match what you see there:"
  echo
  python3 - "$RAW_JSON" <<'PY'
import json, sys
d = json.load(open(sys.argv[1])); s = d["summary"]
rows = [
 ("Third-party (non-Microsoft) apps consented", s.get("thirdPartyApps")),
 ("Admin-consented (tenant-wide) apps", s.get("adminConsentedApps")),
 ("User-consented apps (shadow IT)", s.get("userConsentedApps")),
 ("Apps with application (app-only) permissions", s.get("appsWithApplicationPermissions")),
 ("Apps with privilege-escalation permissions", s.get("appsWithEscalationPermissions")),
 ("High-risk apps (score >= 3)", s.get("highRiskApps")),
 ("Microsoft first-party apps (excluded)", s.get("microsoftFirstParty")),
 ("Total service principals scanned", s.get("totalServicePrincipals")),
]
print("| Count | Value |"); print("| --- | ---: |")
for k,v in rows: print(f"| {k} | {v} |")
print()
print("## Top findings (name -> risk -> flags)")
print()
for a in d["apps"][:15]:
    print(f"- **{a.get('displayName')}** (score {a.get('riskScore')}): {', '.join(a.get('riskFlags', [])) or 'none'}")
PY
  echo
  echo "## Raw command output (the receipt for every count above)"
  echo
  echo '```'
  echo "\$ microsoft-graph-cli apps consent --json"
  echo '```'
  echo '```json'
  cat "$RAW_JSON"
  echo '```'
  echo
  echo "## Auth check (doctor)"
  echo
  echo '```'
  cat "$DOCTOR_TXT"
  echo '```'
} > "$RECEIPTS_MD"
say "  wrote receipts -> $RECEIPTS_MD"

step "Done - the FIRST real Servosity governance report exists"
say "  Report:   $REPORT_MD"
say "  Receipts: $RECEIPTS_MD  (headline counts are spot-checkable in Entra)"
say "  Raw JSON: $RAW_JSON"
