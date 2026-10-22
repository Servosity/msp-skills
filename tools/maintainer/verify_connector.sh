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
#   build+vet               go build + go vet, native             [build]
#   cross-compile           the six released GOOS/GOARCH targets  [build]
#   go test                 the connector's tests, native         [build, new]
#   windows vet             GOOS=windows go vet (test files too)  [windows]
#   windows test compile    GOOS=windows go test -c: every test
#                           binary compiles AND links for Windows [windows]
#   linux test compile      GOOS=linux go test -c (a Mac never builds
#                           *_linux_test.go natively)             [build, new]
#   linux vet               GOOS=linux go vet (a Mac never vets
#                           *_linux.go natively)                  [build]
#   unix-only test lint     check_unix_test_asserts.py            [windows, predicted]
#   cli claims / doctor truth / mcp gate / hand-fix ledger        [build]

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

build_vet() { in_mod go build ./... && in_mod go vet ./...; }

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

# CI's build row runs `go test` on linux/amd64. On a Mac the native `go test`
# never compiles a *_linux_test.go file, so compile and link the Linux test
# binaries too (they run in CI; a Mac cannot run them).
linux_test_compile() { ( cd "$mod" && GOOS=linux GOARCH=amd64 go test -c -o /dev/null ./... ); }
# CI's build row vets on linux/amd64 too; a Mac's native vet never reads a
# *_linux.go file (copylocks and friends live in the vet analyzers, not in
# the compiler).
linux_vet() { ( cd "$mod" && GOOS=linux GOARCH=amd64 go vet ./... ); }

if [ ! -f "$mod/go.mod" ]; then
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector harness" connector "$slug" fail 0 "$logs/harness.log" >> "$rec"
  echo "$mod/go.mod not found" > "$logs/harness.log"
  exit 0
fi

check "build+vet"              build_vet
check "cross-compile"          cross_compile
check "go test"                in_mod go test -count=1 ./...   # as CI: never a cached result
check "windows vet"            windows_vet
check "windows test compile"   windows_test_compile
check "linux test compile"     linux_test_compile
check "linux vet"              linux_vet
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
