#!/usr/bin/env bash
# The check functions run indirectly (through gate/check), which ShellCheck
# cannot follow, so its never-invoked notice is off for this file.
# shellcheck disable=SC2329
# verify_connector.sh - the per-connector half of verify_all.sh. Runs every
# check CI's build and windows matrix rows block on for ONE connector, plus the
# pre-PR checks that predict the windows row before a PR exists. verify_all.sh
# runs one of these per connector in scope, several in parallel.
#
#   bash tools/maintainer/verify_connector.sh <workdir> <slug> <skill-dir>
#
# Writes one record line per check to <workdir>/records/<slug>.tsv and each
# check's output to <workdir>/logs/<slug>/. Always exits 0: the records carry
# the verdict, and a non-zero exit would make xargs stop the other workers.
#
# Checks (CI job that blocks on the same thing, in brackets):
#   linux build+vet         go build + go vet in CI's Linux context
#                           (linux/amd64, cgo on: native, or a container)  [build]
#   linux go test           go test -count=1, same context                 [build, new]
#   cross-compile           the six released GOOS/GOARCH targets            [build]
#   windows vet             GOOS=windows go vet (test files too)            [windows]
#   windows test compile    GOOS=windows go test -c: every test
#                           binary compiles AND links for Windows           [windows]
#   unix-only test lint     check_unix_test_asserts.py                      [windows, predicted]
#   cli claims / doctor truth / mcp gate / hand-fix ledger                  [build]

set -uo pipefail
cd "$(dirname "$0")/../.." || exit 0

work="$1"; slug="$2"; dir="$3"
mod="skills/$dir/cli"
logs="$work/logs/$slug"
rec="$work/records/$slug.tsv"
mkdir -p "$logs" "$work/records"
: > "$rec"

check() { # check <name> <command...>
  name="$1"; shift
  log="$logs/$(printf '%s' "$name" | tr -c 'A-Za-z0-9._-' '_').log"
  t0=$(date +%s)
  if "$@" >"$log" 2>&1; then st=pass; else st=fail; fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$name" connector "$slug" "$st" "$(( $(date +%s) - t0 ))" "$log" >> "$rec"
}

in_mod() { ( cd "$mod" && "$@" ); }

cross_compile() {
  fail=0
  for pair in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
    if ! ( cd "$mod" && GOOS="${pair%/*}" GOARCH="${pair#*/}" go build ./... ); then
      echo "does not compile for $pair"
      fail=1
    fi
  done
  return "$fail"
}

windows_vet() { ( cd "$mod" && GOOS=windows GOARCH=amd64 go vet ./... ); }

# -o /dev/null with several packages: go links every test binary and discards
# it (a link error is a failure too), so nothing lands in the module.
windows_test_compile() { ( cd "$mod" && GOOS=windows GOARCH=amd64 go test -c -o /dev/null ./... ); }

# --- CI's Linux build row, run IN Linux. --------------------------------------
# `Build and vet` and `Test (linux, native)` run on linux/amd64 with cgo on.
# No check of file names or build tags from another OS can stand in for that
# (a test that fails only when GOOS is linux, a cgo preamble that #errors on
# __linux__), so these commands run in that context: natively on a linux/amd64
# host with cgo, else in a linux/amd64 container (golang:<go.mod toolchain>,
# the version setup-go's go-version-file selects in CI), else not at all, and
# the check fails closed. verify_all.sh decides the mode once (VERIFY_LINUX_MODE).
linux_mode="${VERIFY_LINUX_MODE:-none}"

go_version() { # the version CI's setup-go installs: the toolchain line, else the go line
  awk '/^toolchain go/ {print substr($2,3); f=1; exit} END {if (!f) exit 1}' "$mod/go.mod" 2>/dev/null \
    || awk '/^go / {print $2; exit}' "$mod/go.mod"
}

linux_run() { # linux_run <shell command>: run it in skills/<dir>/cli, in CI's Linux context
  # The environment is EXPLICIT, never inherited: no GOFLAGS (a -run=^$ would
  # skip every test), no go env file (GOENV=off), cgo on, CI=true, and the
  # toolchain CI's setup-go selects from go.mod, pinned with GOTOOLCHAIN even
  # where a newer local go would otherwise build with itself.
  local ver
  ver="$(go_version)"
  case "$linux_mode" in
    native)
      ( cd "$mod" && env -i PATH="$PATH" HOME="${HOME:-/tmp}" TMPDIR="${TMPDIR:-/tmp}" \
          GOPATH="$(go env GOPATH)" GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)" \
          GOENV=off GOFLAGS= GOTOOLCHAIN="go$ver" CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CI=true \
          bash -c "$1" ) ;;
    docker)
      # The whole repository as it would be committed (tracked plus untracked,
      # unignored files), snapshotted once by verify_all.sh: repo-level inputs
      # (a Go workspace, a fixture a test reads from ../../..) are there as in
      # CI's checkout, and no test writes into the worktree. Docker could not
      # read a worktree under a protected macOS folder anyway.
      [ -n "${VERIFY_LINUX_SNAPSHOT:-}" ] && [ -d "$VERIFY_LINUX_SNAPSHOT/skills/$dir/cli" ] || {
        echo "no repository snapshot for the Linux container (VERIFY_LINUX_SNAPSHOT)"; return 1; }
      docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
        -e HOME=/tmp -e CI=true -e CGO_ENABLED=1 -e GOTOOLCHAIN=local -e GOENV=off -e GOFLAGS= \
        -e GOMODCACHE=/gomod -e GOCACHE=/gocache \
        -v "${VERIFY_LINUX_GOMOD:-msp-skills-verify-gomod}:/gomod" \
        -v "${VERIFY_LINUX_GOCACHE:-msp-skills-verify-gocache-amd64}:/gocache" \
        -v "$VERIFY_LINUX_SNAPSHOT:/src" -w "/src/skills/$dir/cli" \
        "golang:${ver}-bookworm" bash -c "$1" ;;
    *)
      echo "CI's Linux build row (linux/amd64, cgo on) cannot run here: this host is not"
      echo "linux/amd64 with a C compiler, and no Docker daemon is reachable. Install Docker or"
      echo "OrbStack, or run on Linux. Not a pass: these results come only from a real Linux run."
      return 1 ;;
  esac
}

linux_build_vet() { linux_run 'go version && go build ./... && go vet ./...'; }
linux_test()      { linux_run 'go test -count=1 ./...'; }   # as CI: never a cached result

if [ ! -f "$mod/go.mod" ]; then
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector harness" connector "$slug" fail 0 "$logs/harness.log" >> "$rec"
  echo "$mod/go.mod not found" > "$logs/harness.log"
  exit 0
fi

check "linux build+vet"        linux_build_vet
if [ "${VERIFY_MODULE_ONLY:-0}" = 1 ]; then
  # A module outside the connector set (the full run builds and vets every
  # skills/*/cli, as verify_all.sh always did); CI's matrix has no row for it.
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector complete" connector "$slug" pass 0 /dev/null >> "$rec"
  echo "  done  $slug (module only)"
  exit 0
fi
check "linux go test"          linux_test
check "cross-compile"          cross_compile
check "windows vet"            windows_vet
check "windows test compile"   windows_test_compile
check "unix-only test lint"    python3 tools/maintainer/check_unix_test_asserts.py --slug "$slug"
check "cli claims"             python3 tools/maintainer/check_cli_claims.py --slug "$slug"
check "doctor truth"           python3 tools/maintainer/check_doctor_truth.py --slug "$slug"
check "mcp gate"               python3 tools/maintainer/check_mcp_gate.py --slug "$slug"
check "hand-fix ledger"        python3 tools/maintainer/check_handfixes.py --slug "$slug"

failed=$(awk -F'\t' '$4 == "fail"' "$rec" | wc -l | tr -d ' ')
# The parent accepts this connector's records only when this line exists: a
# worker killed part-way leaves no completion record and fails the verdict.
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector complete" connector "$slug" pass 0 /dev/null >> "$rec"
echo "  done  $slug ($failed failed)"
exit 0
