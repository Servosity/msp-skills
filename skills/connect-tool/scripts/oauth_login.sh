#!/usr/bin/env bash
# Lane A: drive a CLI-owned OAuth loopback login while keeping the token out of
# context. The consuming CLI's own `auth ...-login` runs a 127.0.0.1 callback,
# catches the code, exchanges + stores the token ITSELF - the token never renders
# in the DOM or in this script's stdout. This broker only surfaces the (non-secret)
# authorization URL so the skill can drive the consent click in the bound Chrome,
# then confirms success WITHOUT reading the token.
#
#   RUN_DIR=<dir> oauth_login.sh --start -- <cli login cmd...>   # prints AUTH_URL=...
#   RUN_DIR=<dir> oauth_login.sh --finish                        # prints OAUTH_OK | FAIL
#   oauth_login.sh --selfcheck
#
# The CLI's own output (which may contain the token) goes ONLY to $RUN_DIR/.oauth_cli.log,
# which the model must NEVER cat. This script prints only AUTH_URL / OAUTH_OK.
set -uo pipefail

phase_start() {
  local log="$RUN_DIR/.oauth_cli.log" pidf="$RUN_DIR/.oauth_cli.pid"
  mkdir -p "$RUN_DIR"; umask 077; : > "$log"
  nohup "$@" >>"$log" 2>&1 &
  echo $! > "$pidf"
  local url cand
  for _ in $(seq 1 "${OAUTH_URL_WAIT:-40}"); do
    url=""
    # Keep the authorize URL's required params (client_id/scope/redirect_uri/state/
    # code_challenge) but skip any candidate carrying a secret-bearing param
    # (access_token/code/id_token/refresh_token/client_secret). A real consent URL
    # never has those; a tokened one is a leak - fail closed rather than echo it.
    while IFS= read -r cand; do
      case "$cand" in
        *access_token=*|*id_token=*|*refresh_token=*|*client_secret=*|*"code="*) continue;;
      esac
      url=$cand; break
    done < <(grep -oE 'https://[^ ]*(authorize|oauth|consent|/o/)[^ ]*' "$log")
    [ -n "$url" ] && { echo "AUTH_URL=$url"; return 0; }
    sleep 0.5
  done
  echo "FAIL: no safe authorization URL surfaced within ${OAUTH_URL_WAIT:-40} ticks" >&2; return 3
}

phase_finish() {
  local log="$RUN_DIR/.oauth_cli.log" pidf="$RUN_DIR/.oauth_cli.pid" p
  p=$(cat "$pidf" 2>/dev/null || echo)
  for _ in $(seq 1 "${OAUTH_FINISH_WAIT:-300}"); do
    { [ -n "$p" ] && kill -0 "$p" 2>/dev/null; } || break
    sleep 1
  done
  if [ -n "$p" ] && kill -0 "$p" 2>/dev/null; then
    kill "$p" 2>/dev/null; echo "FAIL: login timed out" >&2; return 4
  fi
  # Confirm from the CLI's OWN words - never echo the log (it may hold the token).
  if grep -qiE 'login (ok|success|stored)|token stored|authenticated|^success' "$log"; then
    echo "OAUTH_OK"; return 0
  fi
  echo "FAIL: login did not confirm (log is local-only, not shown)" >&2; return 6
}

selfcheck() {
  local tmp; tmp=$(mktemp -d)
  # fake CLI login: prints a consent URL, leaks a fake token to ITS stdout, then confirms
  local out
  out=$(RUN_DIR="$tmp" OAUTH_URL_WAIT=20 bash "$0" --start -- \
        bash -c 'echo "open https://accounts.google.com/o/oauth2/auth?client_id=x"; sleep 0.3; echo "SECRET bearer abc123"; echo "token stored"')
  grep -q '^AUTH_URL=https://accounts.google.com/o/oauth2/auth' <<<"$out" \
    || { echo "selfcheck FAIL: consent URL not surfaced ($out)"; exit 1; }
  grep -qi 'abc123' <<<"$out" && { echo "selfcheck FAIL: token leaked from --start"; exit 1; }
  sleep 0.5
  out=$(RUN_DIR="$tmp" OAUTH_FINISH_WAIT=10 bash "$0" --finish)
  grep -q '^OAUTH_OK' <<<"$out" || { echo "selfcheck FAIL: finish not OK ($out)"; exit 1; }
  grep -qi 'abc123' <<<"$out" && { echo "selfcheck FAIL: token leaked from --finish"; exit 1; }
  rm -rf "$tmp"
  # P0 regression: a tokened authorize URL must NOT surface; a clean one alongside wins
  local tmp2 out2; tmp2=$(mktemp -d)
  out2=$(RUN_DIR="$tmp2" OAUTH_URL_WAIT=20 bash "$0" --start -- \
        bash -c 'echo "https://evil.example/oauth/authorize?access_token=LEAK999"; echo "https://accounts.google.com/o/oauth2/auth?client_id=ok&scope=openid"; sleep 0.2; echo "token stored"')
  grep -qi 'LEAK999' <<<"$out2" && { echo "selfcheck FAIL: tokened authorize URL leaked to stdout"; exit 1; }
  grep -q '^AUTH_URL=https://accounts.google.com/o/oauth2/auth' <<<"$out2" \
    || { echo "selfcheck FAIL: clean URL not surfaced alongside tokened one ($out2)"; exit 1; }
  rm -rf "$tmp2"
  echo "oauth_login.sh selfcheck OK (clean+tokened URLs handled, token never echoed, OK confirmed)"
}

case "${1:-}" in
  --selfcheck) selfcheck; exit 0;;
  --start) : "${RUN_DIR:?set RUN_DIR}"; shift; [ "${1:-}" = "--" ] && shift; phase_start "$@";;
  --finish) : "${RUN_DIR:?set RUN_DIR}"; phase_finish;;
  *) echo "usage: oauth_login.sh --start -- <cli login cmd> | --finish   (RUN_DIR required)" >&2; exit 2;;
esac
