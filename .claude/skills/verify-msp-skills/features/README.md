# msp-skills connector verification map

This directory is the maintained source for verifying what a user of any
msp-skills connector actually touches: the installer, the `<slug>-cli` CLI,
and the `<slug>-mcp` server over stdio and HTTP. Read this index before
driving, then use the matching feature file as the recipe. The harness is
`../scripts/verify.py`; every command below runs from the repository root.

## Baseline preconditions

- `python3 .claude/skills/verify-msp-skills/scripts/verify.py doctor --slug <slug>` exits 0.
- The connector is built from this checkout by the run itself
  (`go build` with release.yml's flags); never drive a binary from your PATH.
- Every driven process gets a scratch HOME (`$TMPDIR/verify-msp-<slug>-XXXX/home`)
  with the four XDG dirs inside it, and an environment built from nothing.
- The vendor is a fake on `127.0.0.1`; base-URL and host variables from
  `manifest.json` point at it and credential variables get stand-in values.
- On macOS every driven process runs under the `sandbox-exec` egress guard,
  proved at the start of each run. On Linux pass `--egress-guard proxy-env`.
- Never drive an instance this run did not start, and never touch the user's
  real store (`~/.local/share/<slug>-cli` and friends): the run fingerprints
  those paths before and after and fails (exit 2) if one changed.

## Driving conventions

- One command per feature set: `verify.py run --slug <slug> --features <N,...>`.
  Feature 1 installs the binaries; when it is not selected the run places the
  built binaries in the scratch HOME instead and says so in the summary.
- MCP stdio: one live JSON-RPC session per step, one request at a time, so
  each tool call's vendor traffic is attributed to it.
- MCP HTTP: plain HTTP POSTs to `http://127.0.0.1:<port>/mcp`.
- CLI: subprocess with a timeout; the whole process group is stopped on timeout.
- Env var names (`<PREFIX>_MCP_READ_ONLY`, `<PREFIX>_MCP_HTTP_TOKEN`,
  `<PREFIX>_NO_LOCAL_DATA`) come from the connector's source when it declares
  them, else its manifest, else the prefix of its other variables. The
  summary records which.

## Proof and skip reporting

- Each feature ends in one verdict: PASS, FAIL (harness ran, check failed),
  ERROR (harness broke) or SKIP (no such surface), with a one-line reason.
- CLI proof: argv, env, exit code, stdout, stderr (`f<N>-*/<step>.json`).
- MCP proof: the full JSON-RPC transcript (`*.jsonl`) and server stderr.
- Side-effect proof: `vendor-requests.jsonl` (what reached the vendor),
  files in the scratch HOME (`scratch-home-tree.txt`), exit codes.
- A behavior that could not be reached is ERROR or SKIP with the attempted
  step named, never a PASS through a different path.
- Evidence lives in `.verify-evidence/<slug>/<run>/` and survives cleanup.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the
user-visible behavior, then exactly four H2 sections in this order:
`Sub-features`, `How to get to it (user POV)`, `Driving it with verify.py`
(starting with `Preconditions:`) and `Gotchas`.

## Features

- [Install path](./install-path.md): `install.sh`, `--version`, `--help`, `doctor`.
- [MCP stdio surface](./mcp-stdio.md): `initialize`, `tools/list`, one read call.
- [MCP HTTP transport](./mcp-http.md): token-less default launch, bearer auth.
- [Write gating](./write-gating.md): preview first, send only on `confirm`.
- [Read-only switch](./read-only-switch.md): `<PREFIX>_MCP_READ_ONLY=1` drops write tools.
- [Local data](./local-data.md): store path in `doctor`, `local-data purge`, `<PREFIX>_NO_LOCAL_DATA=1`.

Features 3 to 6 are expected to FAIL on connectors printed before press
4.34.0 and to PASS after a reprint on it; `verify.py report` shows the flip
per slug with the press version beside it.
