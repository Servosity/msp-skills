# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.2] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.1] - 2026-11-09

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).

## [0.1.0] - 2026-11-07

### Added
- Initial msp-skills release: Datto SaaS Protection (Backupify) CLI and MCP server on
  CLI Printing Press 4.34.2, with a local SQLite mirror of every client's domains, seats
  and applications.
- Fleet seat audit: `seats gaps`, `seats reconcile`, `seats summary` and `seats roster`.
- `backup-health` ranks clients by backup coverage and short backup windows.
- `exit-inventory` builds an offboarding manifest with a blockers checklist.
- `seats bulk-plan` plans License, Pause and Unlicense batches without sending anything.
- `seats bulk-change`, the only write: the request is validated (case-sensitive enums,
  at most 100 ids) before it is sent, redirects are refused, and over MCP it previews
  and sends only when `confirm` is boolean `true`.
- `doctor` verifies the key pair against a real read endpoint.
- Over MCP, `sync`, `export`, `teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`,
  `learnings_confirm`, `learnings_forget`, `learnings_reject`, `playbook_amend` and
  `workflow_archive` preview and run only when `confirm` is boolean `true`. Reads such as
  `recall` and `playbook_list` can still add a record to the local learning log.
- The shared credential kit: a credential this program holds, in any encoding, is withheld
  from stdout, stderr, MCP results, files, the local store and any other host.
