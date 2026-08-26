# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
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
  raises the database format number, which means **version 0.1.1 and older can no longer open a
  database this version has opened** - they would put the unused columns straight back. Older
  versions now say so and stop instead of doing that silently. Upgrade every machine that shares a
  database file.
- **Local search for resources returned nothing from the resource-specific index.** With no detailed
  table there was no search index for resources at all, so the connector fell back to the
  general-purpose index. Resources have their own index again, and `search --type resources` now
  reads both it and the general one, so a database synced by an older version keeps answering
  before its first re-sync. Results are matched up by resource id, so a resource whose two copies
  have drifted apart is returned once, using the copy the rest of the connector reads.
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
