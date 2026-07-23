#!/usr/bin/env bash
# Append ONE structured event to the run's events.jsonl (the audit trail).
# Structured for three jobs: troubleshooting (phase/operation/status/error_class),
# review (ordered stream of what did/didn't happen), improvement (patterns.py mines
# error_class across runs). REFUSES to write any secret-shaped field - redaction by
# construction, so no secret value can ever land in the log.
#
#   RUN_DIR=<dir> audit_log.sh event=bind status=ok target=halopsa scheme=api-key phase=preflight
#   audit_log.sh --selfcheck
set -uo pipefail

FORBIDDEN='^(value|secret|token|access_token|refresh_token|api_key|client_secret|bearer|password)$'

emit_event() {
  local run_dir=${RUN_DIR:?set RUN_DIR}
  mkdir -p "$run_dir"
  local ts json k v kv
  ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  json="{\"ts\":\"$ts\""
  for kv in "$@"; do
    k=${kv%%=*}; v=${kv#*=}
    klc=$(printf '%s' "$k" | tr '[:upper:]' '[:lower:]')   # case-insensitive (matches state.py)
    if [[ "$klc" =~ $FORBIDDEN ]]; then
      echo "REFUSING to log forbidden field '$k'" >&2; return 9
    fi
    if printf '%s' "$v" | grep -qiE '(access_token|refresh_token|client_secret|api_?key|password|passwd)["[:space:]]*[:=]'; then
      echo "REFUSING to log value with an embedded secret in field '$k'" >&2; return 9   # defense in depth
    fi
    v=${v//\\/\\\\}; v=${v//\"/\\\"}; v=${v//$'\n'/ }   # json-escape + strip newlines
    k=${k//\\/\\\\}; k=${k//\"/\\\"}; k=${k//$'\n'/ }   # keys too, so the line stays valid JSON
    json="$json,\"$k\":\"$v\""
  done
  json="$json}"
  printf '%s\n' "$json" >> "$run_dir/events.jsonl"
  printf 'logged: %.90s\n' "$json"
}

selfcheck() {
  local tmp; tmp=$(mktemp -d)
  RUN_DIR="$tmp" emit_event event=bind status=ok target=demo scheme=api-key >/dev/null
  grep -q '"event":"bind"' "$tmp/events.jsonl" || { echo "selfcheck FAIL: event missing"; exit 1; }
  grep -q '"status":"ok"' "$tmp/events.jsonl"   || { echo "selfcheck FAIL: status missing"; exit 1; }
  test "$(wc -l < "$tmp/events.jsonl")" -eq 1   || { echo "selfcheck FAIL: not one line"; exit 1; }
  if RUN_DIR="$tmp" emit_event access_token=leak 2>/dev/null; then
    echo "selfcheck FAIL: forbidden field accepted"; exit 1
  fi
  if RUN_DIR="$tmp" emit_event ACCESS_TOKEN=leak2 2>/dev/null; then
    echo "selfcheck FAIL: uppercase forbidden field accepted"; exit 1
  fi
  if RUN_DIR="$tmp" emit_event detail='{"access_token":"leak3"}' 2>/dev/null; then
    echo "selfcheck FAIL: embedded-secret value accepted"; exit 1
  fi
  RUN_DIR="$tmp" emit_event detail="token refresh scheduled" >/dev/null \
    || { echo "selfcheck FAIL: benign detail rejected (false positive)"; exit 1; }
  grep -qE 'leak[0-9]?' "$tmp/events.jsonl" && { echo "selfcheck FAIL: secret leaked to log"; exit 1; }
  rm -rf "$tmp"
  echo "audit_log.sh selfcheck OK"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi
emit_event "$@"
