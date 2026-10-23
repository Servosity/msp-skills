# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `wordpress-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.8] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.7] - 2026-10-15

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

## [0.1.6] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.5] - 2026-09-28

Reprinted on cli-printing-press **4.32.5** (from 4.26.1). Every command and flag that
shipped in 0.1.4 is still here; the list below is what changed for an operator or an
agent.

### Changed
- **Engine 4.32.5.** Runtime and MCP layers regenerated on the current press. Every MCP
  tool-call value is now joined to its flag as one word (`--flag=value`, press #4647) by
  generated code rather than a hand-fix, and the MCP server refuses argument names
  carrying `=` and the local-store destination flags (`--db`, `--output`, `--audit-dir`,
  ...) at the tool schema as well as at run time.
- **Local store schema 4 -> 11.** The first 0.1.5 command that opens the local mirror
  migrates it in place. An older binary cannot reopen a migrated store; re-run `sync`
  if you go back to 0.1.4.
- **`--agent` no longer implies `--yes`.** `--agent` still sets `--json --compact
  --no-input --no-color`; a write that asks for confirmation now needs an explicit
  `--yes`.
- **New commands:** `teach`, `recall`, `learnings`, `playbook` (a local learning loop,
  off with `--no-learn`) and `api` (browse the raw endpoint surface), with matching MCP
  tools. The group-level MCP tools (`posts`, `pages`, `media`, `categories`, `tags`,
  `users`, `settings`, `workflow`), which only returned their own help text, are gone;
  every leaf tool (`posts_list`, `media_update`, `workflow_status`, ...) is unchanged.
- **New global flags:** `--receipt` / `--receipt-file` / `--audit-dir` write a private
  run receipt; `--client-profile` selects a tenant-gated client profile; `--rate-limit`
  now defaults to `auto` (paces to the server's rate-limit headers).
- **Credentials precedence.** When both `config.toml` and the credentials file hold a
  credential, the config file now wins. 0.1.4 never wrote credentials into
  `config.toml`, so this only matters if you put one there by hand.
- **Remote MCP (`--transport http`)** now requires `WORDPRESS_MCP_HTTP_TOKEN` (callers
  send `Authorization: Bearer <token>`) and binds loopback `127.0.0.1:7777` by default;
  a non-loopback `--addr` needs `--tls-cert` and `--tls-key`. `mcp-install.md` and the
  ChatGPT FAQ now show the working command (the old `--addr :7777` line is refused).
- **Three new optional install prompts** on every channel (`.mcpb`, MCP Registry,
  `mcp-install.md`): `WORDPRESS_MCP_HTTP_TOKEN`, `PRINTING_PRESS_CLIENT_PROFILE` and
  `WORDPRESS_USER_AGENT` (override the User-Agent some hosts block).
  `WORDPRESS_BASE_URL` and `WORDPRESS_BASIC_AUTH` are unchanged.
- **Release artifacts:** the `.mcpb` bundle carries the companion `wordpress-cli`, and
  `wordpress-mcp` reports its real version (`0.1.5`) instead of `0.0.0-dev`.

### Fixed
- **`doctor` checks the credential against `/users/me`.** It used to probe the public
  `/categories` route, so a web server that strips the `Authorization` header (a common
  Application Password failure) still read "valid". A site that disables `/users/me`
  gets a not-verified warning instead of a wrong-base_url error.
- **`--config` / `WORDPRESS_CONFIG` keep one credentials file.** `auth set-token` and
  `auth logout` now write and clear the same `data/credentials.toml` beside the config
  that loading reads, including through a symlinked config, so a stale token can no
  longer survive a new `set-token` or a `logout`.
- **MCP `search` and `sql` read the store `sync` wrote.** On a fresh install `sync`
  writes a credential-scoped `data-<hash>.db`; the MCP tools looked only for `data.db`
  and reported no local data.

## [0.1.4] - 2026-09-10

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

## [0.1.3] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **The credential-precedence tests could never pass, so nothing was watching which secret goes on
  the wire.** The four tests that pin the order - a saved credentials file beats an old secret left
  in `config.toml`, a corrupt credentials file falls back to that config and then to the
  environment, an empty one clears nothing - looked for the raw secret inside the `Authorization`
  header. This connector uses HTTP Basic, so the header carries the secret base64-encoded and the
  check could never match, whatever the code did. The tests now decode the header first and assert
  which credential it was built from. Runtime behaviour is unchanged: the precedence order was
  always correct, and is now actually verified.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `WORDPRESS_BASE_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.0] - 2026-07-01

### Changed

- feat(wordpress): add WordPress pages/posts/media connector (#171)

