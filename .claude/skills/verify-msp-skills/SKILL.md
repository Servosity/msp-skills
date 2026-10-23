---
name: verify-msp-skills
description: "Drive one msp-skills connector (skills/<slug>, any of the 84) the way a user does and prove it with evidence: install.sh, the <slug>-cli CLI, and the <slug>-mcp server over stdio and over HTTP, in an isolated HOME against a local fake vendor. Use to check a connector's user path before or after a reprint, to compare press versions across the fleet, or when a connector 'builds green' but nobody has watched it run."
---

# verify-msp-skills

Every connector in this repo is printed by cli-printing-press and shares one
shape: a Go CLI `<slug>-cli` and an MCP server `<slug>-mcp` (stdio by default,
`--transport http` optional) under `skills/<slug>/cli/`, installed by
`skills/<slug>/install.sh`, configured by the env vars in
`skills/<slug>/manifest.json`. So this is ONE skill, parameterized by slug.

It drives the user path and records what happened. It does not repeat CI's
gates; those stay with `tools/maintainer/verify_all.sh` (see Doctor).

Read [features/README.md](features/README.md) before driving. It is the
feature map: six features, each with the observable end state that proves it.

## Launch

There is no long-lived app to start. One command does the whole cycle for a
slug: build, install, drive, capture, clean up.

```bash
python3 .claude/skills/verify-msp-skills/scripts/verify.py run --slug cipp
```

What it starts, in order, all owned by this run:

1. A scratch directory `$TMPDIR/verify-msp-<slug>-XXXX/` holding `home/`
   (the driven processes' HOME, with `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
   `XDG_STATE_HOME`, `XDG_CACHE_HOME` inside it) and `build/`.
2. A fake vendor on `127.0.0.1:<ephemeral>` (in-process). Every base-URL and
   host variable from `manifest.json` points at it; every credential variable
   gets an inert stand-in value. It answers GET with `[]`, a JSON-RPC body
   with a JSON-RPC result, a token request with a stand-in token, anything
   else with `{}`, and logs each request.
3. An egress guard. macOS: every driven process runs under `sandbox-exec`
   with a profile that refuses non-loopback connections; the run first proves
   it (a sandboxed connect to `192.0.2.1` and `2001:db8::1` must fail with
   EPERM). Linux: pass `--egress-guard proxy-env` (proxy variables point at
   the fake vendor, which refuses and logs other hosts; weaker, said so in the
   verdict).
4. `go build -trimpath -ldflags "-s -w -X <module>/internal/cli.version=<v>"`
   for the CLI and `-X main.version=<v>` for the MCP server, as `release.yml`
   does, for this host. This is the artifact under test, not a gate.
5. Per feature: the release fixture
   `tools/maintainer/installer_fixtures/fake_github.py`, MCP servers over
   stdio, and two short-lived HTTP servers. Each is stopped by PID when its
   feature ends.

Ready signals: the fixture writes its port file; the HTTP server accepts a
TCP connect; a stdio server answers `initialize`.

Useful flags: `--features 2,3` (or `mcp-stdio,mcp-http`), `--slug` repeated or
`--all` (one connector after another), `--keep-scratch` (keep `home/` and the
binaries for debugging; remove later with `cleanup`), `--gates` (also run
`verify_all.sh --slug <slug>` and store its verdict with the evidence).

Isolation is real, not assumed: the driven environment is built from nothing
(no inherited variable reaches a connector), and the run fingerprints the
user's real paths for this slug (`~/.config`, `~/.local/share`,
`~/.local/state`, `~/.cache`, `~/Library/Application Support`,
`~/Library/Caches` entries named after the CLI, plus `~/.local/bin/<binaries>`)
before and after. Any change makes the run exit 2.

Two runs can go side by side (separate scratch dirs and ports). Feature 3's
default-address check needs the connector's default port (7777); runs take a
lock for it, and a port held by something else is an ERROR, never a guess.

## Doctor

Read-only. Run it first, and again whenever anything looks off:

```bash
python3 .claude/skills/verify-msp-skills/scripts/verify.py doctor --slug cipp
```

It checks: `go` on PATH, the slug is a connector with `cli/cmd/<slug>-cli` and
`<slug>-mcp`, the manifest's version and `printing_press_version`, the env var
names features 3, 5 and 6 will use (and whether they came from the connector's
source, its manifest, or were derived from its prefix), `install.sh` and the
release fixture, the egress guard (proved, not assumed), `lsof` or `ss`, the
default HTTP port, that `.verify-evidence/` is writable and git-ignored, and
that no earlier run left a process or scratch dir behind. Exit 0 = worth
driving.

Relation to `tools/maintainer/verify_connector.sh`: that script (run through
`bash tools/maintainer/verify_all.sh --slug <slug>`) proves CI's build row and
gates for the source; this skill drives the built binary through the user path
and keeps the evidence, so run both when you need both.

## Drive

The six features, from [features/](features/README.md):

| # | Feature | Drive | Proof |
|---|---|---|---|
| 1 | install path | `install.sh` against the release fixture, then `--version`, `<slug>-mcp --help`, `doctor --json` | binaries installed, sha256 verified, sealed release, commands exit 0 |
| 2 | MCP stdio | JSON-RPC: `initialize`, `tools/list`, one read call | the call reached the fake vendor and answered |
| 3 | MCP HTTP | `--transport http` with no token; then with a token on loopback | refuses or loopback-only; 401 without the token, a session with it |
| 4 | write gating | `<slug>_execute` on a write endpoint, without and with `confirm` | preview and nothing sent; sent only with `confirm: true` |
| 5 | read-only switch | `tools/list` with `<PREFIX>_MCP_READ_ONLY=1` | every listed tool is annotated read-only |
| 6 | local data | `doctor --json`, `local-data purge`, `sync` with `<PREFIX>_NO_LOCAL_DATA=1` | store path named; purge previews; sync refused |

Protocol pieces are reused, not reinvented: the `initialize` handshake,
`result_text`, `is_protocol_error` and `placeholder_args` come from
`tools/maintainer/check_mcp_gate.py`; the manifest-to-env wiring from
`tools/maintainer/class_harness.py`; slugs, binaries and modules from
`tools/maintainer/registry.py`.

## Evidence

Every run writes `.verify-evidence/<slug>/<UTC timestamp>-<pid>/`
(git-ignored; `--evidence-root` moves it):

- `SUMMARY.txt`: one line per feature, the press version, isolation result.
- `verdict.json`: the same, machine-readable, with every sub-check.
- `f<N>-<feature>/`: each command's argv, env, exit code, stdout and stderr
  (`*.json`), each MCP session's full JSON-RPC transcript (`*.jsonl`) and
  server stderr, tool lists, chosen endpoints, HTTP status codes.
- `vendor-requests.jsonl` (every request the connector sent),
  `egress-attempts.jsonl` (requests for any other host, refused),
  `isolation-before.json` / `isolation-after.json`, `scratch-home-tree.txt`
  (what the connector wrote in its isolated HOME).

Proof standards: drive the real binaries through their documented entry
points; record the action and the resulting state (vendor log, files, exit
codes), not only an answer; never call internal setters or test-only hooks.
The fake vendor is the only mock, and it sits where the vendor's network
boundary already is. The HTTP bearer token is random per run and redacted from
every file.

## Verdicts and the reprint flip

Per feature: `PASS` (harness ran, check held), `FAIL` (harness ran, check did
not hold; the observed behavior is the reason), `ERROR` (the harness broke;
says nothing about the connector), `SKIP` (no such surface). `run` exits 0
when everything is PASS or SKIP, 1 for any FAIL, 2 for any ERROR or an
isolation change, 64 on a usage error.

Features 3 to 6 are expected to FAIL on connectors printed before press
4.34.0 and to PASS once reprinted on it. Each verdict records the
`printing_press_version` read from `manifest.json`, so a batch shows the flip:

```bash
python3 .claude/skills/verify-msp-skills/scripts/verify.py run --all
python3 .claude/skills/verify-msp-skills/scripts/verify.py report          # one row per slug
python3 .claude/skills/verify-msp-skills/scripts/verify.py report --json
```

Never loosen a check to turn a FAIL into a PASS; fix the connector or the map.

## Cleanup

Each run cleans up after itself: it stops every process it started (by the
PID it recorded, checked against the command line), stops the fake vendor,
and deletes the scratch directory. Evidence is never deleted, and the run
confirms it still exists after cleanup. For a run that was interrupted:

```bash
python3 .claude/skills/verify-msp-skills/scripts/verify.py cleanup --stale
python3 .claude/skills/verify-msp-skills/scripts/verify.py cleanup --run .verify-evidence/cipp/<run>
```

Never kill by process name: a user's own `<slug>-mcp` may be running.

## Helpers

`scripts/verify.py` (stdlib Python 3, executable). Subcommands: `doctor`,
`run`, `report`, `cleanup`; `--help` on each. Its module docstring states the
verdict and exit-code contract. Keep the map honest as connectors change with
`/maintain-verification-skill`.
