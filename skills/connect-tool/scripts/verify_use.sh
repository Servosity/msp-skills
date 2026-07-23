#!/usr/bin/env bash
# Verify creds by USE - the live receipt. Run a read-only authenticated call and
# assert a NON-SECRET field is present in the response. Never claim "auth works"
# without this. Verify by use, not by printing: the secret is never echoed; the
# proof is a non-secret identifying datum (handle, account email, id).
#
#   verify_use.sh <jq-field> -- <authed read cmd...>
#   e.g. verify_use.sh .email -- halopsa-cli account get
#   verify_use.sh --selfcheck
set -uo pipefail

run() {
  local field=$1; shift
  [ "${1:-}" = "--" ] && shift
  # Case-insensitive: .AccessToken and .access_token are the same refusal.
  local lc; lc=$(printf '%s' "$field" | tr '[:upper:]' '[:lower:]')
  case "$lc" in
    *token*|*secret*|*key*|*bearer*|*password*|*passwd*|*credential*|*cookie*|*session*|*auth*)
      echo "REFUSING to assert on a secret-shaped field ($field)" >&2; return 2;;
    .|"")
      echo "REFUSING to assert on the whole response (use a specific non-secret field)" >&2; return 2;;
  esac
  local backoff=${VERIFY_BACKOFF:-2} out code val
  code=1
  for attempt in 1 2 3; do
    out=$("$@" 2>/dev/null); code=$?
    if [ $code -eq 0 ] && val=$(printf '%s' "$out" | jq -re "$field" 2>/dev/null); then
      echo "RECEIPT_OK field=$field value=$val attempt=$attempt"; return 0
    fi
    sleep $((attempt * backoff))
  done
  echo "RECEIPT_FAIL after 3 attempts (last exit=$code)" >&2; return 1
}

selfcheck() {
  command -v jq >/dev/null || { echo "selfcheck SKIP: jq not installed"; exit 0; }
  VERIFY_BACKOFF=0
  run .data.id -- printf '%s' '{"data":{"id":"acct_00000000"}}' | grep -q RECEIPT_OK \
    || { echo "selfcheck FAIL: good receipt not OK"; exit 1; }
  if run .data.id -- printf '%s' '{}' 2>/dev/null | grep -q RECEIPT_OK; then
    echo "selfcheck FAIL: empty response passed"; exit 1
  fi
  if run .access_token -- printf '%s' '{"access_token":"x"}' 2>/dev/null; then
    echo "selfcheck FAIL: secret-shaped field not refused"; exit 1
  fi
  if run .AccessToken -- printf '%s' '{"AccessToken":"x"}' 2>/dev/null; then
    echo "selfcheck FAIL: mixed-case secret-shaped field not refused"; exit 1
  fi
  if run . -- printf '%s' '{"access_token":"x"}' 2>/dev/null; then
    echo "selfcheck FAIL: whole-response assertion not refused"; exit 1
  fi
  echo "verify_use.sh selfcheck OK"
}

if [ "${1:-}" = "--selfcheck" ]; then selfcheck; exit 0; fi
run "$@"
