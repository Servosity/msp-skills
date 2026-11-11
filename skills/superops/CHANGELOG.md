# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.18] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.17] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).

## [0.1.16] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).

## [0.1.15] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.14] - 2026-11-05

### Security
- Credential protection now runs on the shared credential kit (`internal/credkit`), which replaces this connector's own holder, output check and decoded matching. What changes for you: anything about to leave the program that carries a held credential is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`, on standard output and error, MCP responses over stdio and HTTP, local files, the local store, outbound requests and child processes. A saved profile, cache entry, receipt, delivered file, feedback row or jobs row that would carry one is not written. The learn journal and teach log keep the kit's notice in place of a row that would carry one, instead of a row with the field blanked.
- A credential is no longer sent over plain `http://`, on loopback included. Point `SUPEROPS_BASE_URL` at `https://`, and use `SUPEROPS_CA_CERT` for a private certificate.
- When the MCP server runs the companion CLI, the API token and any setting that carries a credential (a proxy, a base URL with a password) stay out of its environment. The companion must be `superops-cli` beside `superops-mcp`: one reached through `PATH` or `SUPEROPS_CLI_PATH` in another directory is refused while a key is in the environment.
- A credential goes only to the origin this run selected, scheme and port included: with a custom base URL or another region, no `--deliver` webhook, feedback send, proxy or redirect receives one, and the other data center is no exception. An explicit `--config` with no `base_url` selects the default host, not a host a discovered config names.
- The credential fields of a `raw` `--vars` body (from the command line, stdin or an `@file`) and the values of a saved run profile under a credential name are held by the kit as soon as they are read, every duplicate key, number and nested value included, so a `--dry-run` preview does not print them. Free text (a feedback note, a description, a URL in a flag or field that is not a credential) is not scanned for credentials.
- A saved run profile is held at startup, whether or not the run opens it, and a value saved under a name that is no flag of this CLI is neither applied nor printed.
- `SUPEROPS_FEEDBACK_ENDPOINT` is no longer treated as a credential itself.
- The journal offset, the store owner marker, the rate-limit lock and the fingerprint key now pass the credential check like every other local file.
- A URL username of one or two characters beside a password is no longer treated as a credential; the password still is, at any length. Such a short word matches nearly all output, which then disappeared whole.
- Rebuilt on CLI Printing Press 4.34.0 (advisory GHSA-jh26-vhqh-692c). The MCP server's HTTP mode now refuses to start without `SUPEROPS_MCP_HTTP_TOKEN`, checks a bearer token on every request, serves `http://127.0.0.1:7777/mcp` by default and needs TLS for any other address. The interim "do not run HTTP mode" note is removed.
- `raw mutation` now sends only with `--yes`; without it, it prints the request and sends nothing. Over MCP it can only preview. `raw query` refuses any document that is not a query.
- Over MCP, GraphQL variables go inline in the document; the separate `vars` field is no longer offered, and an `@path` document is still refused.
- GraphQL error text (SuperOps answers errors with HTTP 200) now passes the same credential check as HTTP errors, as do sync, archive and doctor error lines.
- The local store and the sync checkpoint are now kept per tenant (token, subdomain and host). After this upgrade the first `sync` starts a fresh store; a store that cannot prove its tenant is never reused.
- A local store now records its tenant (subdomain and host, never the token) and is refused for any other tenant, including a store named with `--db`: two tenants syncing into one `--db` file no longer mix rows. A store that records no tenant is never read by the CLI or by the MCP `sql` and `search` tools, whether or not it holds data (such as a `data.db` from an earlier release, read with no token set). A command records its tenant in a store only while the file holds no data at all, and records it before writing anything, so of two tenants opening one new file only one gets it. The tenant a store is checked against comes from the same configuration the command's requests use, read once per command. The refusal names the remedy: sync this tenant into a fresh `--db` path or into the default store, or, for an empty file, run `sync` with the same `--db`. Cached GET responses are also kept per subdomain.
- The tenant identity the CLI keeps locally (the store's tenant record and the sync checkpoint) holds only the API host and path, never a username, password or query string from `SUPEROPS_BASE_URL`.
- A username or password in `SUPEROPS_BASE_URL` is no longer shown in any output: `doctor` (human and `--json`), error messages, retry notices, `--dry-run` previews, sync events, the jobs ledger, and MCP tool results and errors show the URL as scheme, host, port and path only. A URL that does not parse is shown as `[url withheld: it does not parse as scheme://host]`.
- A `SUPEROPS_BASE_URL` with an `@` (or its percent-encoded form) outside its username and password, for example a password holding an unescaped `/`, is now refused when the configuration loads, with a message that does not show it. Before, part of the password was read as the host or path, so it could be shown or stored locally. Percent-encode any `/`, `?`, `#` or `@` in the username or password. A base URL with a query string or fragment is refused the same way: it could never reach the API, and request errors showed it.
- HTTP mode now refuses a `SUPEROPS_MCP_HTTP_TOKEN` shorter than 32 characters or holding anything but letters, digits and `-._~+/=`, so an unfilled placeholder or unexpanded shell text is never accepted as the token. `openssl rand -hex 32` gives a valid one.
- A credential shorter than 8 characters (the API token, an auth header token, or a base URL password) is now refused when the configuration loads, so nothing is sent with it.
- Every credential the CLI and MCP server hold is now kept out of error output, whatever its source and length: config file values including the `[headers]` table, the credentials file (also when it is refused for its file mode), a token read by `auth set-token`, `SUPEROPS_API_TOKEN`, `SUPEROPS_MCP_HTTP_TOKEN`, and any username or password in `SUPEROPS_BASE_URL`, a proxy variable or a saved `base_url`. Before, a `[headers]` value such as `Authorization = "Bearer ..."` echoed back in a SuperOps error was shown. An error that repeats one of them, as written or encoded, is withheld; a very short value (a 2-letter header or username) withholds more error text.
- The same check now covers every byte the CLI and MCP server write, success output included: everything on standard output and standard error from any writer (command output, warnings, retry notices, sync events, `--dry-run` and `--deliver` notices, Go and mcp-go log lines), every MCP message over stdio and every MCP HTTP response. A credential split across two lines or two writes is still caught, and a JSON key is checked like a value. A withheld MCP response becomes an error to the same request, so a client never waits on it. The writes that do not reach the screen pass it too: a `--deliver` copy is withheld the same way, a store write (rows, sync checkpoints, the tenant record) that repeats a credential is refused, a saved profile and feedback (kept locally or sent upstream) are refused, the learn journal and the teach and audit logs skip the entry, and receipts, the jobs ledger and the response cache are not written. A vendor record that repeats a credential is never stored: `sync` refuses that page with `records not stored: a vendor record repeats a credential this program holds`. Because there is no length minimum, a very short credential (a 2 to 4 letter username or header value that is also an ordinary word) withholds more output and can refuse store writes; use a longer value if that gets in the way. Before, `doctor` could show a probe code that was a short username's base64, `sync` stored a ticket whose text repeated the API token, a `--vars` key, a password holding a newline or a flag named after a token could be shown or journaled, and mcp-go's log line could show the HTTP token used as a session id.
- `doctor --fail-on error` now fails on an `UNAUTHENTICATED` probe even when a short held username appears inside the code; only the codes it displays are withheld.
- A tenant named only by a `[headers]` `CustomerSubDomain` value is now that tenant everywhere: the value becomes the subdomain the store records, so two such configs no longer share a store. A `[headers]` `CustomerSubDomain` that disagrees with the `subdomain` setting is refused; `SUPEROPS_SUBDOMAIN` replaces both.
- A username or password in a proxy setting is now also held when it is written without a scheme (`user:password@proxy.example:8080`). An email address has no password and is not treated as a credential.
- A config, credentials or legacy file that does not parse is still read for credentials at startup, token by token.
- A redirect to another host is refused when its URL, a header or a re-sent body would carry a credential. Before, a `[headers]` value such as `X-Api-Key` followed a redirect to another host.
- The learn journal no longer keeps a `SUPEROPS_LEARN_SESSION` value as written (it is hashed, so a session still groups), keeps only the flag names the command declares (any other is recorded as `<undeclared>`, and nothing after `--` counts as a flag), and skips an entry whose query, label or operand repeats a credential. A `teach` or `playbook amend` with no `--query` logs counts, not its arguments.

### Fixed
- `sync` now pages through every SuperOps list page (it stopped after the first 50 rows) and reports a list whose end it cannot prove as incomplete.
- Every resource is stored under its SuperOps id, so records with the same name no longer collapse into one row, and a plain `sync` now covers all 14 resources the local commands read.
- `doctor` checks the real GraphQL endpoint and no longer reports a healthy default install as unconfigured.
- `sync --dry-run` and the local commands under `--dry-run` preview instead of failing or printing nothing.
- The local commands open the store read-only, report a store read error instead of skipping rows, and never create a store under `SUPEROPS_NO_LOCAL_DATA=1`.

### Changed
- New optional settings on every install channel: `SUPEROPS_MCP_READ_ONLY`, `SUPEROPS_NO_LOCAL_DATA` and `SUPEROPS_MCP_HTTP_TOKEN`.
- Removed `api` and `import` (neither worked against this GraphQL API; `raw` covers both) and the root `--idempotent` flag. `tail` is hidden.

## [0.1.13] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.12] - 2026-10-22

### Security
- Over MCP, `raw query` and `raw mutation` now refuse a positional GraphQL document given as `@path` (a local file the server would read); pass the document inline instead. Other commands still accept an `@`-leading value such as a client name, and the command line still accepts `@path`.

## [0.1.11] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.
- `raw query` and `raw mutation` accept GraphQL variables over MCP only as an inline JSON object, not an @path file.

## [0.1.10] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.9] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.8] - 2026-09-28

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first for this connector to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)
- No connector code changed in this release; it exists so the published bundle and
  binaries pick up the release-pipeline fixes above.

## [0.1.7] - 2026-09-10

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

## [0.1.6] - 2026-08-26

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

## [0.1.5] - 2026-08-26

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
  authenticate. Now declared on every install channel: `SUPEROPS_BASE_URL`, `SUPEROPS_REGION`, `SUPEROPS_SUBDOMAIN`.

## [0.1.4] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.3] - 2026-06-17

### Changed

- fix(superops): restore CustomerSubDomain header + EU region routing (#132) (#134)

## [0.1.2] - 2026-06-16

### Changed

- docs(superops): consolidate 0.1.1 fix into the 0.1.2 release section (#114)
- fix(superops): request scalar GraphQL fields without sub-selections (#114) (#126)
- fleet: re-vendor 48 connectors to printing-press 4.24.0 engine (#96)
- chore(fleet): press-version provenance + back-fill hand-fix ledgers (#91)
- fix: drop false "with sound" demo claim, highlight first-party Servosity, point backup/DR skills at Servosity (#77)
- fix(install): honor GITHUB_TOKEN/GH_TOKEN in fetch_stdout across all skills (#31)

## [0.1.0]

### Added
- Initial msp-skills release: `superops-cli` + `superops-mcp` covering the SuperOps
  PSA+RMM GraphQL surface - tickets, assets, alerts, clients, sites, users, contracts,
  invoices, worklogs, technicians, service items, IT docs, and KB - as typed
  `list`/`get` subcommands and MCP tools.
- Cross-entity views the console can't compose in one screen: `sla-watch` (open tickets
  breaching or near their resolution SLA, grouped by tech or client), `client-360` (one
  client's sites, users, contracts, open tickets, assets, and open invoices), `unbilled`
  (billable logged worklog totaled per client - the month-end reconciliation target),
  `at-risk-assets` (endpoints both unpatched and actively alerting), `alert-coverage`
  (alerts split resolved vs unresolved per client), `stale-tickets` (open tickets idle
  past N days), and `context-ticket` (one ticket + worklogs + client + SLA as an
  agent-shaped, `--select`-friendly bundle).
- Offline SQLite sync (`sync`, incremental and resumable; access-denied resources are
  warnings, not failures) with FTS5 full-text `search` and `analytics` over synced data.
- Read-only by design: every typed command reads; the single write path is `raw mutation`
  (with `raw query` for reads), and `--dry-run` previews the exact GraphQL request.
- Agent ergonomics: `--agent` mode (JSON, non-interactive), `--select`/`--compact` field
  control, `--deliver` output sinks, named `profile` flag sets, `which` capability lookup,
  and `doctor` for an auth/connectivity health check.
