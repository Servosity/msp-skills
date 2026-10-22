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
# toolchain setup-go selects>) on a Linux disk holding a depth-1 clone of this
# commit with the working tree's changes laid on top, else not at all, and they
# fail closed. Every command runs with an EXPLICIT environment: nothing Go reads
# (GOFLAGS, GOAMD64, GO111MODULE, a go env file...) is inherited from the caller.
#
# The WINDOWS row runs on a real Windows runner, which no other OS can be. Its
# predictors run on this host under the same explicit environment, on a copy
# of that job's SPARSE checkout (skills/<slug>, tools/maintainer and the files
# directly in the root and skills/), so a go.work or a replace directive that
# reaches outside it fails here as it does there: Windows vet, a Windows
# compile and link of every test, and the Unix-only test lint.
# They build with cgo off and CI's runner builds with cgo on (it has MinGW), so
# they speak for it only when turning cgo on changes nothing Go compiles (any
# file of any kind, in the module or a dependency, either way); otherwise
# "windows cgo context" fails closed.
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
  # A result that cannot be recorded ends the worker WITHOUT its completion
  # record, which the parent treats as a failure.
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$name" connector "$slug" "$st" "$(( $(date +%s) - t0 ))" "$log" >> "$rec" \
    || { echo "  cannot record $name for $slug in $rec" >&2; exit 0; }
}

# The Go CI's rows install: setup-go reads go.mod's `go` line (a bare 1.26 is
# its newest patch). The go command then selects as usual, a newer `toolchain`
# line in go.mod or go.work upgrading it, so every Go command here runs with
# GOTOOLCHAIN=go<that>+auto: the same selection. Resolved once, at the repo root.
gover="$(python3 tools/maintainer/verify_scope.py ci-go --gomod "$mod/go.mod" \
  ${VERIFY_GO_RELEASES:+--releases "$VERIFY_GO_RELEASES"} 2>/dev/null | cut -f1 | sed 's/^go//')"
# The go.work CI's checkout hands Go for this module (repo-relative), or off:
# never one above the repository or an ignored one, which CI does not have.
gowork="$(python3 tools/maintainer/verify_scope.py gowork --dir "$mod" 2>/dev/null)"
work_at() { [ "$gowork" = off ] && echo off || echo "$1/$gowork"; }  # work_at <repo root as seen there>

# CI's runners start from a clean Go environment, and so does this script
# (verify_all.sh does the same): every GO* and CGO_* variable and the C
# toolchain go away, except those that only say where modules, caches and
# downloads live, which never change what is built.
for v in $(compgen -e); do
  case "$v" in
    GOPATH|GOMODCACHE|GOCACHE|GOTMPDIR|GOBIN|GOPROXY|GONOPROXY|GOPRIVATE|GONOSUMDB|GOSUMDB|GOINSECURE|GOAUTH|GOVCS|GOTELEMETRY*|GOOGLE_*) ;;
    GO*|CGO_*|CC|CXX|AR|PKG_CONFIG|GCCGO) unset "$v" ;;
  esac
done
export GOENV=off GOWORK=off

# CI's Windows checkout, and the go.work Go finds inside it (or off).
win="$work/win/$slug"
winwork=off
[ "${VERIFY_MODULE_ONLY:-0}" = 1 ] \
  || winwork="$(python3 tools/maintainer/verify_scope.py windows-tree --slug "$slug" --out "$win" 2>/dev/null)"
wingo() { # wingo VAR=value... command...: on the Windows job's toolchain and workspace
  env GOFLAGS= GOWORK="$([ "$winwork" = off ] && echo off || echo "$win/$winwork")" \
    GOTOOLCHAIN="go$gover+auto" CI=true "$@"
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
      [ -n "${VERIFY_LINUX_SRC:-}" ] && [ -d "$VERIFY_LINUX_SRC/$mod" ] || {
        echo "no repository snapshot for the Linux checks (VERIFY_LINUX_SRC)"; return 1; }
      ( cd "$VERIFY_LINUX_SRC/$cwd" && env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" CI=true \
          ${GOPATH:+GOPATH="$GOPATH"} ${GOMODCACHE:+GOMODCACHE="$GOMODCACHE"} ${GOCACHE:+GOCACHE="$GOCACHE"} \
          ${GOPROXY:+GOPROXY="$GOPROXY"} \
          GOOS=linux GOARCH=amd64 CGO_ENABLED=1 GOENV=off GOFLAGS= GOWORK="$(work_at "$VERIFY_LINUX_SRC")" \
          GOTOOLCHAIN="go$gover+auto" bash -c "$cmd" ) ;;
    docker)
      [ -n "${VERIFY_LINUX_SRC:-}" ] || {
        echo "no repository snapshot for the Linux container (VERIFY_LINUX_SRC)"; return 1; }
      # The container's environment is only what is listed here. No -w: Docker
      # hands a volume's working directory back to root, and this user then
      # cannot write there.
      docker run --rm --platform linux/amd64 --user "${VERIFY_LINUX_USER:-$(id -u):$(id -g)}" \
        -e HOME=/tmp -e CI=true -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=1 \
        -e GOTOOLCHAIN=auto -e GOENV=off -e GOFLAGS= -e GOWORK="$(work_at /src)" \
        -e GOMODCACHE=/gomod -e GOCACHE=/gocache \
        -v "${VERIFY_LINUX_GOMOD:-msp-skills-verify-gomod}:/gomod" \
        -v "${VERIFY_LINUX_GOCACHE:-msp-skills-verify-gocache-amd64}:/gocache" \
        -v "$VERIFY_LINUX_SRC:/src" \
        "golang:${gover}-bookworm" bash -c 'cd "/src/$1" && eval "$2"' bash "$cwd" "$cmd" ;;
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
windows_vet()          { ( cd "$win/$mod" && wingo GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./... ); }
# -o /dev/null with several packages: go links every test binary and discards
# it (a link error is a failure too), so nothing lands in the module.
windows_test_compile() { ( cd "$win/$mod" && wingo GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /dev/null ./... ); }

windows_cgo_context() {
  wingo GOOS=windows GOARCH=amd64 python3 tools/maintainer/verify_scope.py cgo-context --dir "$win/$mod" && return 0
  echo "CI's Windows runner builds with cgo on (it has MinGW) and the Windows checks here build"
  echo "with cgo off, so they do not speak for it. Rely on CI's Windows job for this connector."
  return 1
}

# The lint evaluates build tags for the Go CI's Windows job selects (release
# tags go1.1..go1.N), so it asks Go which one that is.
unix_lint() {
  local v
  v="$(cd "$win/$mod" && wingo GOOS=windows GOARCH=amd64 go env GOVERSION)" || { echo "go env GOVERSION failed: $v"; return 1; }
  python3 tools/maintainer/check_unix_test_asserts.py --root "$win" --slug "$slug" --go-version "$v"
}
if [ ! -f "$mod/go.mod" ] || [ -z "$gover" ] || [ "$gover" = - ] || [ -z "$gowork" ] || [ -z "$winwork" ]; then
  # No module, or no way to know which Go CI installs for it: nothing below
  # can speak for CI, so the connector fails here, visibly.
  if [ ! -f "$mod/go.mod" ]; then echo "$mod/go.mod not found"
  elif [ -z "$gowork" ]; then echo "cannot tell which go.work CI's checkout uses for $mod"
  elif [ -z "$winwork" ]; then echo "cannot build a copy of CI's sparse Windows checkout for $slug"
  else echo "cannot resolve the Go CI installs for $mod/go.mod (setup-go reads its go line; a bare X.Y needs go.dev's release index)"
  fi > "$logs/harness.log"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector harness" connector "$slug" fail 0 "$logs/harness.log" >> "$rec"
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
check "unix-only test lint"    unix_lint

failed=$(awk -F'\t' '$4 == "fail"' "$rec" | wc -l | tr -d ' ')
# The parent accepts this connector's records only when this line exists: a
# worker killed part-way leaves no completion record and fails the verdict.
printf '%s\t%s\t%s\t%s\t%s\t%s\n' "connector complete" connector "$slug" pass 0 /dev/null >> "$rec"
echo "  done  $slug ($failed failed)"
exit 0
