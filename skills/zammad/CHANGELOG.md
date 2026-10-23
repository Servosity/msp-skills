# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.10] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `zammad-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.9] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.8] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.1.7] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.6] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.28.0). Every command, flag and
MCP tool that shipped in 0.1.5 is still here; the list below is what changed for
an operator or an agent.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. The two
  security fixes this connector carried by hand since September are now generated code
  and no longer depend on a hand-fix surviving a reprint: every MCP tool-call value is
  joined to its flag as one word (`--flag=value`, the 0.1.5 fix, press #4647) and the
  MCP server refuses argument names carrying `=` and the local-store destination flags
  (`--db`, `--output`, `--audit-dir`, ...) at the schema as well as at run time.
- **`--agent` no longer implies `--yes`.** `--agent` still sets `--json --compact
  --no-input --no-color`; a command that asks for confirmation now needs an explicit
  `--yes`. Scripts that relied on `--agent` to auto-confirm a write must add `--yes`.
- **New global flags:** `--receipt` / `--receipt-file` / `--audit-dir` write a private
  run receipt; `--client-profile` selects a tenant-gated client profile;
  `--rate-limit` now defaults to `auto` (paces to the server's rate-limit headers).
- **MCP tool surface:** `kb_browse`, `kb_get`, `kb_search`, `articles_sync` and
  `zammad_get` are now exposed as tools, and the `read_the_kb_before_answering` recipe
  is registered. The three no-op parent tools (`learnings`, `playbook`, `workflow`),
  which only returned their own help text, are gone. `stale`, `orphans` and `load` are
  unchanged.
- **Three new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `ZAMMAD_MCP_HTTP_TOKEN` (bearer token, only for
  `--transport http`), `PRINTING_PRESS_CLIENT_PROFILE` (client-profile binding) and
  `ZAMMAD_USER_AGENT` (overrides the User-Agent sent to Zammad; blank keeps the
  built-in one). `ZAMMAD_URL`, `ZAMMAD_BASE_URL` and `ZAMMAD_API_TOKEN` are unchanged.
- **Remote (HTTP) launch recipe.** `mcp-install.md` and the ChatGPT answer now say
  `ZAMMAD_MCP_HTTP_TOKEN=<value> ... zammad-mcp --transport http` with no `--addr`:
  the server listens on `127.0.0.1:7777` by default and refuses a non-loopback
  `--addr` (such as the old `--addr :7777`) unless `--tls-cert` and `--tls-key` are
  given, and refuses to start `--transport http` without the bearer token. Put the
  loopback listener behind your HTTPS tunnel or reverse proxy as before.
- **Release artifacts:** first zammad release cut after the 2026-09-18 pipeline change,
  so the `.mcpb` bundle carries the companion `zammad-cli` and `zammad-mcp` reports its
  real version (`0.1.6`) instead of `0.0.0-dev`.

### Fixed
- **`ZAMMAD_URL` kept working.** The 4.32.5 generation dropped the hand-wired read of
  `ZAMMAD_URL` (instance root, `/api/v1` appended), which every install doc names as the
  way to point the CLI at your instance; the read is restored, pinned by a test and now
  recorded in the hand-fix ledger.
- **`stale`, `orphans` and `load` kept working.** The 4.32.5 profiler no longer emits
  the three project-management workflow commands for this API; they are carried as
  hand-authored files so the CLI and MCP surfaces stay the same.
- **MCP `--transport http` listener** now sets a request-header read deadline, so one
  client cannot hold the listener open by dribbling headers.
- **`doctor`, `recall` / `playbook list` honesty fixes preserved** (0.1.3 / 0.1.4):
  the placeholder-`base_url` refusal, the explicit no-credential row and the
  `mcp:local-write` annotations were re-applied on the fresh tree; the doctor
  information-key handling is now generated.

## [0.1.5] - 2026-09-10

### Fixed
- **A tool-call value could smuggle a refused flag past the MCP server.** The MCP server
  handed each tool argument to the CLI as two separate words, the flag and then its value.
  On a yes/no flag the CLI does not read the next word as a value, so a value that itself
  began with `--` was read as a brand-new flag, including flags the server deliberately
  refuses such as `--deliver`, which can send command output to a URL. Every argument that
  carries a value now travels glued to its flag as one word (`--flag=value`), so the CLI only
  ever reads it as the value of that one named flag, or rejects it, and it can never become a
  second flag. A plain yes/no flag is still a bare `--flag`, and an empty value is still left out.
  Reported privately through SECURITY.md; the same fix is in the pending DataGate connector.

## [0.1.4] - 2026-09-01

### Fixed
- **`recall` and `playbook list` no longer claim to be read-only.** Both open the writable
  learn store and record a row, but were annotated `mcp:read-only=true` - and that annotation
  is what an MCP host reads to decide what to auto-approve without asking you. They are now
  `mcp:local-write`, a tier this engine already defines and already uses for `teach`: writes
  land only in the CLI's own local store, never in external state and never in a user-visible
  file. Measured at the live MCP server, both tools moved from `readOnlyHint=true,
  openWorldHint=true` to `readOnlyHint=false, destructiveHint=false, openWorldHint=false`.
  No behaviour changed - both commands write exactly what they wrote before. The promise was
  the defect, not the write.

### Changed
- Install and remote-agent documentation corrected against the shipped binaries. The remote
  section named `mcp-remote`, which bridges the opposite direction and cannot publish a local
  stdio server at all; connectors that parse `--transport http` now point at the native flag
  and the rest at `supergateway`. The HTTP endpoint is `/mcp`, not the bare root the docs gave.
  The Windows install path and a fallback paragraph describing an `npx` install that is not
  offered were both wrong and are gone. A new `check_install_docs` gate holds these claims
  against the binaries and installers so they cannot drift again.

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://your-instance.zammad.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `ZAMMAD_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ZAMMAD_BASE_URL`, `ZAMMAD_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.0]

### Added
- Initial msp-skills release: Zammad CLI + MCP server with an offline SQLite mirror.
- Full ticket surface: list, get, search (Zammad query syntax), create, update, delete, plus articles (read/add) and a one-line `ticket note`.
- Knowledge Base: browse, search, and read answers (parsed from the init bundle) plus create/publish/set-internal/delete.
- Team-management analytics the API can't answer in one call: `agent-load`, `agent-trend`, `customer-health`, `overdue`, `escalate`, `churn-risk`, and `feedback-scan`.
- Reference reads: organizations, users, groups, states, priorities, tags, overviews.
- Per-instance config (`ZAMMAD_URL` + `ZAMMAD_API_TOKEN`); works with any self-hosted or hosted Zammad.
