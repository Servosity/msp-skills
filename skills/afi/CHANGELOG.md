# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.12] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.11] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.10] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.9] - 2026-11-05

### Changed
- Rebuilt on the current press. The MCP server's HTTP mode now listens on localhost by default, refuses to start without `AFI_MCP_HTTP_TOKEN`, and needs TLS for any other address; every MCP write previews first and sends only with `confirm: true`; `AFI_MCP_READ_ONLY=1` refuses writes; `AFI_NO_LOCAL_DATA=1` keeps no local copy; `local-data purge` erases what is stored.
- **`backup-stale` now reports what the API can prove.** It flags protected resources with no archive at all, and protected resources in a tenant whose synced task statistics show no finished task. An archive's `created_at` dates its backup sequence, not its latest backup, so `--max-age` is hidden and ignored with a note.
- `offboard` trusts the finished backup task, refuses a task that ended in `warnings`, and refuses a trigger that returns no task id.
- `orgs create` prints the `invitation_url` redacted; Afi emails the invitation to `admin_email`.
- `import` is CLI-only: it reads a local file, which an MCP tool argument can never name.

### Fixed
- `fleet-sync` removes what the API no longer returns, once a list was read in full: a protection released by `offboard` or in the portal no longer reads as protected. A list it could not read in full keeps its rows and is recorded, and `coverage-gaps`, `fleet-health`, `backup-stale`, `reconcile-licenses`, `tenant-scorecard` and `resolve` name those gaps (`incomplete` and `note`).
- `sync --resources applications` and `workflow archive` store installations (they stored none).
- `doctor` no longer calls the real `papi.afi.ai` host a placeholder, so a default install gets its key probed.
- `resolve --kind` filters before `--limit`, so a match is no longer dropped by the limit.

### Security
- The key goes only to the scheme, host and port of `https://papi.afi.ai` (or `AFI_BASE_URL`), and only in the `Authorization` header; a redirect to another host is refused, and a request that would carry the key anywhere else is refused before it is sent. Responses are never cached on disk.

## [0.1.8] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `afi-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.7] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.6] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.5] - 2026-10-15

### Changed
- **Rebuilt with Go 1.26.9 (was 1.26.6).** Go 1.26.9 includes upstream Go security
  fixes, among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and
  GO-2026-6612. The connector's own code is unchanged.
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but this connector's previous `.mcpb` contained only the MCP
  server, so a one-click install on a machine without the CLI failed at the first tool
  call. This release's bundle ships both binaries side by side for macOS (universal),
  Windows x64 and Linux x64.
- **The MCP server reports its real version.** `serverInfo.version` read `0.0.0-dev` in
  the previous release because the release build stamped the version into the CLI
  only. The release now stamps both and checks the built server's `initialize` reply
  against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256
  sidecars before touching anything, and replace the old binaries in a single
  transaction that is undone if any step fails. (Served from the repository, so this
  already applies to every install, not only this version.)

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
- **Every sync of the `resources` collection warned and stored nothing in its own table.**
  Afi has a collection literally named `resources`, and the generated database gave its
  table that exact name - the same name the connector already uses for its general-purpose
  mirror of every record. The general-purpose table won, so each sync printed
  `warning: N/N resources items: typed-table upsert failed` and the per-collection table stayed
  empty, which is what offline lookups by tenant read from. The per-collection table is now
  named `tenants_resources`, matching how the connector already names `orgs_tenants`.

  What this means for an existing database: every record you have already synced is still in
  the general-purpose table and still returned by `list`, `search` and `sql` - the rename
  touches nothing there and no existing database needs converting. The new
  `tenants_resources` table starts empty and fills on your next successful `sync`. Rows for
  records the vendor has since deleted will not reappear in it, because a sync only writes
  what the API still returns; those stay readable in the general-purpose table.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

## [0.1.1] - 2026-08-26

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
  authenticate. Now declared on every install channel: `AFI_BASE_URL`.

### Changed
- Regenerated the vendored `afi-cli` / `afi-mcp` source from cli-printing-press
  4.24.0 and re-rendered the templated skill surfaces. No changes to command
  names or flags; `fleet-sync`, the fleet-wide coverage/staleness reports, and
  the offboard helper behave as before.

## [0.1.0]

### Added
- Initial msp-skills release: the `afi-cli` CLI and `afi-mcp` MCP server for
  Afi SaaS backup (Microsoft 365 / Google Workspace).
- `fleet-sync` walks the whole Afi hierarchy - installations, orgs, tenants,
  then each tenant's resources, protections, policies, archives, quotas, and
  task stats - into a local SQLite store in one respectful, rate-limited pass.
- Fleet-wide reports that answer offline: `coverage-gaps` (resources with no
  protection), `backup-stale` (protected-but-silently-failing), `fleet-health`
  (all-tenant task + quota rollup), `tenant-scorecard` (one tenant's posture),
  and `reconcile-licenses` (purchased vs protected seats).
- `resolve` maps a Microsoft 365 / Google Workspace ID, email, or name to the
  canonical Afi resource or tenant, including Multi-Geo fan-out.
- `offboard` runs a guarded archive-then-release: it triggers a final backup,
  verifies a fresh archive landed, and only then removes the protection.
- Full public-API coverage via the friendly top-level commands and the `api`
  passthrough, plus `--agent` mode (JSON, non-interactive) for AI agents.
