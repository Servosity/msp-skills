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
#   security gate context   no cgo-only file the local gate cannot scan     [security-gate]
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
  case "$linux_mode" in
    native)
      ( cd "$mod" && CGO_ENABLED=1 CI=true bash -c "$1" ) ;;
    docker)
      # Docker cannot read a worktree under a protected macOS folder, and a
      # test must not write into the worktree anyway: run on a private copy
      # of the whole skill directory (tests may read ../README.md and such).
      local copy ver
      copy="$work/linux/$slug"
      rm -rf "$copy"; mkdir -p "$copy/skills"
      cp -R "skills/$dir" "$copy/skills/" || return 1
      ver="$(go_version)"
      docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
        -e HOME=/tmp -e CI=true -e CGO_ENABLED=1 -e GOTOOLCHAIN=local \
        -e GOMODCACHE=/gomod -e GOCACHE=/gocache \
        -v "${VERIFY_LINUX_GOMOD:-msp-skills-verify-gomod}:/gomod" \
        -v "${VERIFY_LINUX_GOCACHE:-msp-skills-verify-gocache-amd64}:/gocache" \
        -v "$copy:/src" -w "/src/skills/$dir/cli" \
        "golang:${ver}-bookworm" bash -c "$1" ;;
    *)
      echo "CI's Linux build row (linux/amd64, cgo on) cannot run here: this host is not"
      echo "linux/amd64 with cgo, and no Docker daemon is reachable. Install Docker or OrbStack,"
      echo "or run on Linux. Not a pass: these results come only from a real Linux run."
      return 1 ;;
  esac
}

linux_build_vet() { linux_run 'go version && go build ./... && go vet ./...'; }
linux_test()      { linux_run 'go test -count=1 ./...'; }   # as CI: never a cached result

# The security gate itself still runs on this host (its scanners live here),
# with GOOS=linux GOARCH=amd64 but cgo OFF from a non-Linux host. A file only
# a cgo-on build selects would be compiled above but never scanned, so that
# fails closed. Read from `go list -json`, which keeps every file name whole.
security_context() {
  [ "$linux_mode" = native ] && { echo "native linux/amd64 with cgo: the gate reads what CI's gate reads"; return 0; }
  ( cd "$mod" && python3 - <<'PY'
import json, os, subprocess, sys

def files(env):
    p = subprocess.run(["go", "list", "-e", "-json", "./..."], env=dict(os.environ, **env),
                       capture_output=True, text=True)
    if p.returncode != 0 and not p.stdout:
        print(p.stderr)
        sys.exit(2)
    out, dec, s, i = set(), json.JSONDecoder(), p.stdout, 0
    while True:
        while i < len(s) and s[i].isspace():
            i += 1
        if i >= len(s):
            return out
        obj, i = dec.raw_decode(s, i)
        for key in ("GoFiles", "CgoFiles", "TestGoFiles", "XTestGoFiles"):
            for f in obj.get(key) or []:
                out.add(os.path.join(obj.get("Dir", ""), f))

ctx = {"GOOS": "linux", "GOARCH": "amd64"}
missing = sorted(files(dict(ctx, CGO_ENABLED="1")) - files(dict(ctx, CGO_ENABLED="0")))
if missing:
    print("files CI's security gate (linux/amd64, cgo on) scans that this host's gate cannot:")
    for m in missing:
        print("  " + m)
    sys.exit(1)
print("no cgo-only files: this host's gate reads every file CI's gate reads")
PY
  )
}

if [ ! -f "$mod/go.mod" ]; then
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector harness" connector "$slug" fail 0 "$logs/harness.log" >> "$rec"
  echo "$mod/go.mod not found" > "$logs/harness.log"
  exit 0
fi

check "linux build+vet"        linux_build_vet
check "linux go test"          linux_test
check "cross-compile"          cross_compile
check "windows vet"            windows_vet
check "windows test compile"   windows_test_compile
check "security gate context"  security_context
check "unix-only test lint"    python3 tools/maintainer/check_unix_test_asserts.py --slug "$slug"
check "cli claims"             python3 tools/maintainer/check_cli_claims.py --slug "$slug"
check "doctor truth"           python3 tools/maintainer/check_doctor_truth.py --slug "$slug"
check "mcp gate"               python3 tools/maintainer/check_mcp_gate.py --slug "$slug"
check "hand-fix ledger"        python3 tools/maintainer/check_handfixes.py --slug "$slug"
rm -rf "$work/linux/$slug"

failed=$(awk -F'\t' '$4 == "fail"' "$rec" | wc -l | tr -d ' ')
# The parent accepts this connector's records only when this line exists: a
# worker killed part-way leaves no completion record and fails the verdict.
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector complete" connector "$slug" pass 0 /dev/null >> "$rec"
echo "  done  $slug ($failed failed)"
exit 0
