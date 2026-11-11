# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
