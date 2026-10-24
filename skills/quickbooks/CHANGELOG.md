# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.0] - unreleased

Fixes #491, #492, #493.

### Security
- The learn journal keeps no typed text that could be a credential. `QUICKBOOKS_LEARN_SESSION` is stored as a hash, sessions still group, `argv_shape` records only flag names the command declares, a mistyped flag is kept as a placeholder and a hash, so a correction is only learned when the CLI suggested the flag, and `teach.log` records argument counts, not arguments.
- **Rebuilt on Printing Press 4.34.0** ([GHSA-jh26-vhqh-692c](https://github.com/Servosity/msp-skills/security/advisories/GHSA-jh26-vhqh-692c)). The MCP server now splits reads from writes: `quickbooks_read` calls read-only endpoints (`query.run` and every entity list, all served by `GET /query`), and `quickbooks_execute` returns a preview of a state-changing call until the agent passes `confirm: true`. `quickbooks-mcp --transport http` refuses to start without `QUICKBOOKS_MCP_HTTP_TOKEN`, answers only requests that send `Authorization: Bearer <token>`, binds `127.0.0.1:7777` by default and needs `--tls-cert`/`--tls-key` for any other address. `QUICKBOOKS_MCP_READ_ONLY=1` lists only read-only tools.
- Every credential this program holds (the environment, the config file, the credentials store, a token answer, a password or credential query value inside `base_url` or an endpoint variable, and a saved profile's value under a credential flag or `base-url`) reaches no output, file, local store or other host. This is the shared credential kit: output that repeats one, or an 8-character piece of one, in any encoding the kit reads, is withheld whole with a notice. Read every URL on its own, so a link with no credential is left alone.
- A request that a redirect sends to another host carries none of the configured `[headers]`. A credential is sent only over `https`, and only to the base URL, the QuickBooks API hosts or, for the OAuth client and refresh token, Intuit's token endpoint. `QUICKBOOKS_BASE_URL` over plain `http` with a credential is refused, and `QUICKBOOKS_TOKEN_URL` may name only Intuit's endpoint or the base URL's origin.
- `profile save` refuses a webhook URL and any value under a credential flag. `profile show` and `profile use` show a webhook an earlier release saved as `webhook:<saved, not shown>`; it still delivers. No message names a webhook or feedback URL, whose path is its key.
- `quickbooks-cli auth refresh` sends the client secret and refresh token only to an `https` token URL, never follows a redirect, and shows only the HTTP status and the standard OAuth error code when the token endpoint refuses.
- The automatic token refresh (below) holds every value of the token answer as it is read, refused answers included, so a new access or refresh token reaches no output, and a token endpoint's error body is reduced to the HTTP status and the standard OAuth error code. Only the tokens are saved (a client secret set only in the environment is not copied to disk), through a temporary file in the same directory (created exclusively, mode 0600, flushed to disk, then renamed), and the refresh's lock file refuses a symlink.

- A credential is bound only to the base URL the command uses: a default or legacy config it does not read, an overridden `--config` or `--home`, a config the program distrusts, or the server a credentials store was saved for no longer widens where it may go, and the other QuickBooks API host is no longer open by default.
- A `--stdin` body is held before anything prints, on a `--dry-run` preview as on a send. The body is held whole, so output that repeats any of it is withheld, a create's response included: the request is still sent, so check the record before retrying, or use `--body-json` to see the response. `feedback --stdin` now refuses and asks for the note as an argument, since a held note could never reach the ledger. A saved profile's value under a credential flag or `base-url` is held from the start of the run, and a credential field in an MCP call is held before the tool answers.
- A credential inside `QUICKBOOKS_TOKEN_URL` or `QUICKBOOKS_FEEDBACK_ENDPOINT` is held before the startup refusal can print it, also when the parameter name is percent-encoded. A webhook or feedback error no longer prints the URL.

### Added
- `report cash-flow`: the Statement of Cash Flows, with the same period flags as `report pnl` (`--start`, `--end`, `--month`, `--cash`; accrual by default). `metrics` holds net cash from operating, investing and financing activities, the net cash change, and cash at the beginning and end of the period (each null when the report has no such line); `metrics_by_column` repeats them per column. A read-only MCP tool (`report_cash_flow`). (#491)
- `--columns total|month|quarter` on `report pnl`, `report balance-sheet` and `report cash-flow`: one amount per period, with each column's title and dates in `columns[]`. `report balance-sheet --start` sets the first period for a balance per period end. (#493)
- Every report returns `rows[]`: each header, line and total in QuickBooks' order with `kind` (header, account or total), `depth`, `parent` and per-column `amounts`, so a group total is told apart from an account and the layout QuickBooks shows can be rebuilt. `--raw` prints QuickBooks' own Header/Columns/Rows JSON. The existing `accounts`, `summaries` and `metrics` fields are unchanged. (#493)
- `bills due`: open bills due in the next `--days` days (default 7) plus every overdue bill, earliest first, with per-vendor totals. Bills with no due date are listed under `undated`.
- `invoices unsent`: open invoices QuickBooks has neither emailed nor printed, largest balance first, with `has_bill_email`. Invoices that report no `EmailStatus` are counted in `status_unreported`.
- `QUICKBOOKS_MCP_READ_ONLY`, `QUICKBOOKS_NO_LOCAL_DATA`, `QUICKBOOKS_MCP_HTTP_TOKEN` and `QUICKBOOKS_USER_AGENT` are declared on every install channel (Claude Desktop bundle, MCP Registry, manual config).

### Fixed
- The access token is refreshed automatically: before a request when it is missing or expires within 60 seconds, and once after a 401 (a second 401 is returned, never looped). QuickBooks rotates the refresh token on refresh; the new access token, refresh token and expiry are saved to the credential store before the request is retried, and a refresh whose tokens cannot be saved fails the request rather than going ahead. Concurrent refreshes (a CLI run and the MCP server, or several requests at once) take one cross-process lock, so they make one token call between them and the rest use the token it saved. The new tokens are saved to the credentials file the credentials were read from (a `--config` profile keeps its own). The stored refresh token is used before `QUICKBOOKS_REFRESH_TOKEN`, which is tried when the stored one is rejected. `auth refresh` runs the same locked refresh. (#492)
- `auth set-token`, `auth login` and `auth logout` with `--config` use the credentials store a token refresh of that profile uses, under its lock: a token set by hand lands where the next run looks, and a logout signs out that profile and no other.
- A token refresh that stops after saving the rotated tokens and before rewriting an older config file that still holds credentials no longer brings those older credentials back on the next run. An edit to that config file afterwards still takes precedence.
- `sync` finishes cleanly on an entity with no records and on a book whose size is an exact multiple of 1,000. QuickBooks answers both with an empty `QueryResponse`; a `Fault` in the answer still fails that entity.
- Money totals in a multi-currency book use the home-currency amount (the home field, else amount times `ExchangeRate`). `balances` returns `by_currency` and leaves the cross-currency totals null when the records span more than one currency; `customers top` ranks within each currency; rows carry `currency` and `foreign_balance` when foreign. Single-currency books are unchanged.
- The local mirror is named for the QuickBooks company (the Intuit host and realm), so it is still there after an access-token refresh, for the CLI and the MCP server alike. Each company keeps its own mirror. A base URL that names no company (no `/v3/company/<realm>`) uses the unclaimed `data.db` and never claims it, so the next run with the realm still finds the books an earlier release synced.
- Due dates and aging are counted in calendar days in your own time zone, and `report balance-sheet` defaults to today's local date.
- The MCP server's `search` and `sql` tools read the same local file as the CLI when `QUICKBOOKS_DB_PATH` is set.

### Removed
- `export <resource>`. QuickBooks Online serves its lists only through `/query`, so the command could not return data. Use `<resource> list --json`, or `sync` then `search` or `sql`.

## [0.1.14] - 2026-10-23

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers the token endpoint errors of `quickbooks-cli auth refresh` too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.13] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.12] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.11] - 2026-10-13

### Changed
- Built with Go 1.26.9 (was 1.26.6). This fixes three Go `net/http` security advisories that `govulncheck` found reachable in the previous build: GO-2026-6617 (an HTTP/2 crash), GO-2026-6613 and GO-2026-6612. No commands, flags or output changed.

## [0.1.10] - 2026-10-02

### Fixed

- Expose executor request schemas in MCP endpoint discovery, including required inputs, wire-key case, nested bodies, and known enum values. Search now finds request fields and exact short enum values (#351).
- Preserve separate path, query, header, and body inputs, including endpoints that reuse a name in more than one location. Existing CLI commands and custom workflows are retained.

## [0.1.9] - 2026-09-28

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

## [0.1.8] - 2026-09-10

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

## [0.1.7] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **The novel-command acceptance tests failed on the calendar, not on a code change.** Their
  seeded books used fixed 2026 dates while the commands under test read today's clock, so once
  the calendar passed the newest seeded due date every invoice looked overdue: the cash-forecast
  test saw 800 of overdue inflows instead of 500 and an entirely empty eight-week window, and the
  stale-invoice test saw a not-yet-due invoice join the collections list. The seeded dates are now
  whole-day offsets from the day the test runs, so the books hold the same shape on any future
  date and in any time zone. Test-only - no shipped behaviour changed; `cash-forecast`,
  `invoices stale` and the aging commands always forecast from the real current date. The tests
  are also stricter now that "today" is known: they assert the overdue split, the beyond-window
  amounts, the net inside the window, and which forward week each open item buckets into.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.6] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  The credential was never checked at all: the report said `present, not verified` and left you to
  guess. `doctor` now issues one authenticated GET against a real read endpoint and reports what came
  back, so an expired token reads as rejected and a wrong base URL reads as a wrong base URL.
  It also dialled the shipped placeholder base URL (`https://quickbooks.api.intuit.com/v3/company/YOUR_REALM_ID`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `QUICKBOOKS_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `QUICKBOOKS_BASE_URL`, `QUICKBOOKS_CLIENT_ID`, `QUICKBOOKS_CLIENT_SECRET`, `QUICKBOOKS_ENVIRONMENT`, `QUICKBOOKS_REALM_ID`, `QUICKBOOKS_REFRESH_TOKEN` and 1 more.

## [0.1.5] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.4] - 2026-06-26

### Changed

- feat(quickbooks): report pnl + report balance-sheet (v0.1.4) (#163)
- chore(quickbooks): redact real tenant transaction counts + add CI guard (#162)

## [0.1.3] - 2026-06-16

### Fixed
- `sync` now pages through the entire book instead of stopping at 1000 rows per
  entity. QuickBooks Online serves every entity through `/query` and caps a page
  at 1000 (`MAXRESULTS`), and the generic sync loop broke after the first page,
  so large books were silently truncated (e.g. only the first 1,000 rows of a
  large book). Because QBO returns rows oldest-first, the missing rows were the
  *recent* ones, making `ar-aging`, `ap-aging`, `dso`, and `balances` wrong, not
  merely partial. `sync` now advances QBO's in-query `STARTPOSITION` until a short
  page. Verified live against a production tenant: all eight resources sync to
  their exact `count(*)` across the full book. Recorded as hand-fix `qbo-query-paging`.
- `aging-delta` no longer errors `no such table: aging_snapshots`. The command
  reads and writes an `aging_snapshots` table that no store migration created, so
  it failed on first use. It now creates the table on demand. Recorded as hand-fix
  `qbo-aging-snapshots-table`.

### Added
- `QUICKBOOKS_DB_PATH` environment override for the local SQLite mirror, so a
  single machine can keep separate sandbox and production mirrors. QBO entity IDs
  are per-company and collide across companies, so one shared `data.db` would
  corrupt aggregates after switching environments. Recorded as hand-fix
  `qbo-db-path-env`.

## [0.1.2] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.1] - 2026-06-13

### Changed

- fix(quickbooks): sync injects /query SQL so the offline mirror hydrates (v0.1.1) (#90)
- fix: drop false "with sound" demo claim, highlight first-party Servosity, point backup/DR skills at Servosity (#77)

## [0.1.0]

### Added
- Initial msp-skills release: QuickBooks Online CLI + MCP server.
- Full Accounting entity coverage: accounts, items, customers, vendors, invoices,
  bills, payments, and journal entries (list / get / create / update / delete).
- Offline SQLite mirror via `sync`, with incremental and full resync, plus FTS5
  `search` across every synced entity.
- Receivables and payables intelligence: `ar-aging`, `ap-aging`, `invoices stale`,
  `payments unapplied`, `customers top`, and `vendors spend`.
- Cash and collections KPIs: `balances`, `cash-forecast`, `dso`, and
  `customer-profitability`.
- Month-end hygiene: `reconcile`, `dupes`, `journal-entries check`, and `aging-delta`
  cross-run snapshot diffing.
- Raw `query` passthrough to the QBO query endpoint, agent-native `--json` / `--select`
  / `--agent` output, and `--dry-run` previews on every write.
