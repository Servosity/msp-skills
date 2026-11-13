# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource Hook refuses this token (HTTP 403) is now recorded as skipped instead of half-finished. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed for any other reason still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling, not what the sync achieved. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.0] - 2026-11-11

### Added
- Every read-only endpoint of Hook's API v1 as a CLI command and an MCP tool:
  organizations and their summaries, campaigns and results, enrollments and batches,
  courses, people, groups and billable users.
- `client-risk`: clients ranked by phishing click rate, with a fleet rate computed from
  totals.
- `clicked-untrained`: people who clicked and have not finished training, with course
  status and due date.
- `overdue-training`: overdue training grouped by client, with `--counts-only` to leave
  out names and emails.
- `qbr`: one client's quarterly review data in one command.
- Optional local SQLite copy through `sync`, with `search` and `sql`.

### Security
- The shared credential kit holds every credential from process start and withholds any
  output, file write or outbound request carrying one, in any encoding.

### Fixed
- `qbr --days N` now covers exactly N days, counting the end date.
- `doctor` reports a 403 from the credential probe as a warning, so `--fail-on warn` sees it.
