# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **`sql "SELECT name, email FROM resources"` answered a row of NULLs per synced resource instead
  of the resource's name and email.** Resource Guru has an API resource literally named
  `resources`, which is also the name of the local database's general-purpose table, so the
  detailed table the connector meant to build for resources was never created - the general one
  already had the name. The 21 detail columns (`name`, `email`, `first_name`, `bookable`,
  `job_title` and the rest) were then added to the general table, where nothing ever filled them,
  so a query for them succeeded and returned empty values rather than saying the columns were not
  there. The detailed table is now called `accounts_resources` - named after the account the
  collection belongs to, the same way `resources_bookings` and `clients_bookings` already are - and
  it is filled on every sync. Querying those columns on the general `resources` table now reports
  that they do not exist.
- **Local search for resources, and offline lookups of a resource's bookings, returned the wrong
  rows.** With no detailed table there was no search index for resources, and the code that finds a
  resource's id was reading the general table, which holds every synced record of every kind - so
  the bookings lookup was fed client and project ids as if they were resource ids. Resources have
  their own search index again, and id lookups read the detailed table.
- Existing databases keep working and keep everything already synced; the detailed table starts
  empty and fills on the next `sync`. The unused columns on the general table are left in place -
  they are empty and nothing reads them, so removing them would risk data for no gain.
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
