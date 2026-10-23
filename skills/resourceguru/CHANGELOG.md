# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.7] - 2026-10-23

### Fixed
- **Windows: two processes saving credentials at the same time no longer lose a save.** Moving the new credentials file into place failed with "Access is denied" while another process had the file open, so one of two saves could fail. The save now retries that step briefly (about half a second at most, only on Windows), as the newer connectors already do.

### Changed
- `cli/NOTICE` now names the product `resourceguru-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.6] - 2026-10-22

### Security
- The MCP server drops the flags whose values name a file or directory on the machine that runs it (for example --db, --output) from its tool schemas and refuses them by name, instead of advertising them and ignoring them silently. The command line keeps every flag.

## [0.1.5] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.4] - 2026-10-15

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

## [0.1.3] - 2026-09-10

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

## [0.1.2] - 2026-08-26

### Fixed
- **An agent could point this connector's local database at any file on the machine.**
  The MCP server forwarded a `db` argument straight through to `sync`, and the store runs a
  migration that drops and rebuilds its tables. A tool call naming another application's SQLite
  file would therefore rewrite that file. The MCP surface now refuses arguments that name a
  filesystem location - by name, and by what the flag's own help text says it does, so a newly
  generated path flag is refused before anyone has to notice it. Nothing an agent could
  legitimately call changed.

- **The `sql` tool answered a row of empty values per synced resource instead of the resource's
  name and email.** Asking it for `SELECT name, email FROM resources` returned one row per record
  with nothing in either column. Resource Guru has an API resource literally named
  `resources`, which is also the name of the local database's general-purpose table, so the
  detailed table the connector meant to build for resources was never created - the general one
  already had the name. The 21 detail columns (`name`, `email`, `first_name`, `bookable`,
  `job_title` and the rest) were then added to the general table, where nothing ever filled them,
  so a query for them succeeded and returned empty values rather than saying the columns were not
  there. The detailed table is now called `accounts_resources` - named after the account the
  collection belongs to, the same way `resources_bookings` and `clients_bookings` already are - and
  it is filled on every sync. Querying those columns on the general `resources` table now reports
  that they do not exist.
- **Databases created by 0.1.0 and 0.1.1 are repaired on the next open, not just going forward.**
  The previous fix stopped new databases from being polluted but left every existing one answering
  empty values forever. Opening an existing database now removes those 20 unused columns and their
  five indexes from the general table, so it answers exactly the way a fresh install does.
  Everything already synced is kept: the full record for every resource is stored as JSON in a
  column the repair never touches, and each column is checked to be empty before it is removed, so
  a column that somehow does hold a value is kept and reported rather than dropped. The repair also
  raises the database format number, which means **version 0.1.1 and older can no longer sync into a
  database this version has opened** - they would put the unused columns straight back. Older
  versions now say so and stop instead of doing that silently. Upgrade every machine that shares a
  database file.
- **The version check now runs when the database is opened for reading, not only for writing.**
  Version 0.1.1 and older check the database format number only on the read-write path, which is the
  path that syncs. Their read-only path - the per-record-type read commands run with
  `--data-source local` or `auto`, and the MCP `search` and `sql` tools - attached to a newer
  database without checking and answered from it. Not every read command took that path: the CLI's
  own `search` command opens the database read-write, so the older check did fire there.
  Nothing can change what an already-released version does, and what it does is limited: a read-only
  connection runs no upgrade step, so it cannot put the removed columns back; the worst case is that
  it reads a database shaped differently than it expects, where a query for one of the removed
  columns now fails outright instead of returning empty values. From this version on, the read path
  performs the same check and stops with the same upgrade message, so a future release cannot read a
  database it does not understand. Databases created before the check existed still open normally.
- **Searching resources could return an out-of-date copy of a record, or the same record twice.**
  Resources now have their own search index again, alongside the general-purpose one the connector
  had been falling back on. Searching read both and tried to reconcile them, and that could not be
  made to work: the two indexes are updated separately, so when a sync only partly succeeds they
  can hold different versions of the same record. A search for a word that only survived in the
  out-of-date version returned that version as though it were current, and a search without
  `--type` returned the record twice, once from each index. Searching now reads only the
  general-purpose index for resources, which is the one every other part of the connector reads
  and the one that always holds the result of the last successful sync - so a database synced by
  an older version still answers before its first re-sync, a record whose two copies have drifted
  apart is returned once using the current copy, and two resources in different accounts that
  happen to share an id are both still returned. The resource-specific index is still built and
  kept up to date for anyone querying the database directly with the `sql` tool. One deliberate
  trade-off: the general-purpose index leaves out text it treats as non-searchable - bare
  identifiers, timestamps, and notes fields that contain nothing but a web address - so a search
  for a word inside one of those no longer matches. That is the same rule every other record type
  in this connector already follows, and it is what version 0.1.1 did too.
- The three lookup helpers that turn a resource type into a database table (used when syncing
  child records) now resolve `resources` to the detailed table rather than to the general one.
  This is hardening rather than a bug fix: nothing in this connector reaches those helpers with
  `resources` today, because the bookings-per-resource sync reads the general table directly and
  filters by record type. An earlier draft of this entry said that sync was being handed client and
  project ids; that was wrong, and a test now pins the real behaviour so the correction stays true.
- **The credential-precedence tests were skipped, so nothing was watching which secret goes on the
  wire.** The four tests that pin the order - a saved credentials file beats an old secret left in
  `config.toml`, a corrupt credentials file falls back to that config and then to the environment,
  an empty one clears nothing - looked for the raw secret inside the `Authorization` header and set
  only the email half of this connector's email plus password pair. They were skipped rather than
  corrected, which left the precedence chain untested. The fixtures now supply both halves and the
  assertion decodes the header, so all four run and were checked to fail when the order is broken.
  Runtime behaviour is unchanged.

### Changed
- Every source file now carries one project copyright line (`Copyright 2026 Servosity Inc. and msp-skills contributors`) instead of the ten different strings the fleet had accumulated; individual contributor credit moved to the repository `NOTICE`. Source headers only, no behaviour changed.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `RESOURCEGURU_BASE_URL`.

## [0.1.0]

### Added
- Initial msp-skills release: the full Resource Guru API as typed CLI commands plus an MCP server.
- Per-day utilization analytics computed locally - `utilization` (booked-vs-available per resource per day, with `--heatmap`), `overbooked` (fleet-wide resource-days over capacity), `bench` (under-utilized resources), and `capacity` (remaining bookable minutes).
- Offline SQLite mirror via `sync`, with `search`, `load`, `stale`, `orphans`, and `since` reading it without extra API calls.
- Agent-native surface: `--agent` mode, `--select` field projection, `--dry-run` previews, and a provenance envelope on store/API reads.
