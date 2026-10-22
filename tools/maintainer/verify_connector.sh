#!/usr/bin/env bash
# The check functions run indirectly (through check), which ShellCheck cannot
# follow, so its never-invoked notice is off for this file.
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
# CI's BUILD row is one Linux job (ubuntu, linux/amd64, cgo on, a depth-1
# checkout): build and vet, cross-compile, CLI claims, hand-fix ledger, MCP
# gate, doctor and go test all run there. So all of them run HERE in that
# context too, never approximated from another OS: natively on a linux/amd64
# host with a C compiler, else in a linux/amd64 container (golang:<the go.mod
# toolchain setup-go selects>) over a depth-1 clone of this commit with the
# working tree's changes laid on top, else not at all, and they fail closed.
# Every command runs with an EXPLICIT environment: nothing Go reads (GOFLAGS,
# GOAMD64, GOEXPERIMENT, a go env file...) is inherited from the caller.
#
# The WINDOWS row runs on a real Windows runner, which no other OS can be. Its
# predictors run on this host under the same explicit environment: Windows
# vet, a Windows compile and link of every test, and the Unix-only test lint;
# a file only a cgo-on Windows build selects (CI's runner has MinGW) fails
# closed, because a cross-check from here builds with cgo off.
#
#   [build]    linux build+vet, linux cross-compile, linux go test, linux cli
#              claims, linux doctor truth, linux mcp gate, linux hand-fix ledger
#   [windows]  windows vet, windows test compile, windows cgo context,
#              unix-only test lint (a predictor; CI's Windows job is the authority)

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

# The version CI's setup-go installs: the toolchain line, else the go line.
# Read once, here at the repo root (the checks below run from other folders).
gover="$(awk '/^toolchain go/ {print substr($2,3); f=1; exit} END {if (!f) exit 1}' "$mod/go.mod" 2>/dev/null \
  || awk '/^go / {print $2; exit}' "$mod/go.mod" 2>/dev/null)"

# Everything the go command reads from the environment that could change
# which files it builds or how: cleared, then set explicitly per context.
GO_ENV_UNSET="GOFLAGS GOAMD64 GOARM GOARM64 GO386 GOMIPS GOMIPS64 GOPPC64 GORISCV64 GOWASM
GOEXPERIMENT GODEBUG GOFIPS140 GOENV GOWORK GOTOOLCHAIN GOOS GOARCH CGO_ENABLED CGO_CFLAGS
CGO_CPPFLAGS CGO_CXXFLAGS CGO_FFLAGS CGO_LDFLAGS CC CXX"

goclean() { # goclean VAR=value... command...: the command under an explicit Go environment
  local u=()
  for v in $GO_ENV_UNSET; do u+=(-u "$v"); done
  env "${u[@]}" GOENV=off GOFLAGS= GOTOOLCHAIN="go$gover" CI=true "$@"
}

# ---- CI's build row, in Linux ------------------------------------------------
linux_mode="${VERIFY_LINUX_MODE:-none}"

linux_run() { # linux_run <dir relative to the repo root> <shell command>
  local cwd="$1" cmd="$2"
  case "$linux_mode" in
    native)
      # Like the container: only what is listed here (no TZ, LANG or test
      # settings from the caller), plus the PATH that finds go and gcc and
      # where modules and the build cache live (which never change a result).
      ( cd "$cwd" && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" CI=true \
          ${GOPATH:+GOPATH="$GOPATH"} ${GOMODCACHE:+GOMODCACHE="$GOMODCACHE"} ${GOCACHE:+GOCACHE="$GOCACHE"} \
          ${GOPROXY:+GOPROXY="$GOPROXY"} \
          GOOS=linux GOARCH=amd64 CGO_ENABLED=1 GOENV=off GOFLAGS= GOTOOLCHAIN="go$gover" bash -c "$cmd" ) ;;
    docker)
      [ -n "${VERIFY_LINUX_SNAPSHOT:-}" ] && [ -d "$VERIFY_LINUX_SNAPSHOT/$mod" ] || {
        echo "no repository snapshot for the Linux container (VERIFY_LINUX_SNAPSHOT)"; return 1; }
      # The container's environment is only what is listed here.
      docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" \
        -e HOME=/tmp -e CI=true -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=1 \
        -e GOTOOLCHAIN=local -e GOENV=off -e GOFLAGS= \
        -e GOMODCACHE=/gomod -e GOCACHE=/gocache \
        -v "${VERIFY_LINUX_GOMOD:-msp-skills-verify-gomod}:/gomod" \
        -v "${VERIFY_LINUX_GOCACHE:-msp-skills-verify-gocache-amd64}:/gocache" \
        -v "$VERIFY_LINUX_SNAPSHOT:/src" -w "/src/$cwd" \
        "golang:${gover}-bookworm" bash -c "$cmd" ;;
    *)
      echo "CI's build row (linux/amd64, cgo on) cannot run here: this host is not linux/amd64"
      echo "with a C compiler, and no Docker daemon is reachable. Install Docker or OrbStack,"
      echo "or run on Linux. Not a pass: these results come only from a real Linux run."
      return 1 ;;
  esac
}

# The cross-compile step exactly as ci.yml runs it on the Linux runner: each
# target's default cgo setting there (on only for linux/amd64, the host).
# shellcheck disable=SC2016 # expanded by the bash inside the Linux context
CROSS='fail=0
for pair in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  os="${pair%/*}"; arch="${pair#*/}"
  cg=0; [ "$pair" = linux/amd64 ] && cg=1
  if ! GOOS="$os" GOARCH="$arch" CGO_ENABLED="$cg" go build ./... 2>build.err; then
    echo "does not compile for $pair"; sed "s/^/    /" build.err; fail=1
  fi
done
rm -f build.err
exit "$fail"'

linux_build_vet()   { linux_run "$mod" 'go version && go build ./... && go vet ./...'; }
linux_cross()       { linux_run "$mod" "$CROSS"; }
linux_test()        { linux_run "$mod" 'go test -count=1 ./...'; }   # as CI: never a cached result
linux_py()          { linux_run . "python3 tools/maintainer/$1 --slug $slug"; }

# ---- CI's windows row: predictors on this host ------------------------------
windows_vet()          { ( cd "$mod" && goclean GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./... ); }
# -o /dev/null with several packages: go links every test binary and discards
# it (a link error is a failure too), so nothing lands in the module.
windows_test_compile() { ( cd "$mod" && goclean GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /dev/null ./... ); }

# A cgo-only Windows file (CI's runner builds with MinGW, cgo on) is in
# neither prediction above, which build with cgo off: fail closed. File sets
# come from `go list -json` under the same explicit environment.
windows_cgo_context() {
  ( cd "$mod" && goclean GOOS=windows GOARCH=amd64 python3 - <<'PY'
import json, os, subprocess, sys

def files(cgo):
    p = subprocess.run(["go", "list", "-e", "-json", "./..."], env=dict(os.environ, CGO_ENABLED=cgo),
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

missing = sorted(files("1") - files("0"))
if missing:
    print("files CI's Windows runner (cgo on, MinGW) builds that this host's Windows checks cannot:")
    for m in missing:
        print("  " + m)
    print("rely on CI's Windows job for this connector, or check it on Windows")
    sys.exit(1)
print("no cgo-only Windows files: the Windows predictors read every file CI's runner builds")
PY
  )
}

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
check "linux cross-compile"    linux_cross
check "linux cli claims"       linux_py check_cli_claims.py
check "linux hand-fix ledger"  linux_py check_handfixes.py
check "linux mcp gate"         linux_py check_mcp_gate.py
check "linux doctor truth"     linux_py check_doctor_truth.py
check "linux go test"          linux_test
check "windows vet"            windows_vet
check "windows test compile"   windows_test_compile
check "windows cgo context"    windows_cgo_context
check "unix-only test lint"    python3 tools/maintainer/check_unix_test_asserts.py --slug "$slug"

failed=$(awk -F'\t' '$4 == "fail"' "$rec" | wc -l | tr -d ' ')
# The parent accepts this connector's records only when this line exists: a
# worker killed part-way leaves no completion record and fails the verdict.
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector complete" connector "$slug" pass 0 /dev/null >> "$rec"
echo "  done  $slug ($failed failed)"
exit 0
