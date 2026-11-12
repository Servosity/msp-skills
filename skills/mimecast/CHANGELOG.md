# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.4] - 2026-11-12

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.3] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.2] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: `mimecast-cli` and the `mimecast-mcp` MCP server
  for Mimecast API 2.0 (OAuth2 client credentials), built on CLI Printing Press
  4.34.2.
- `why`: one verdict for one message (held, rejected at the gateway, released or
  rejected from hold, delivered, bounced or not found) with the policy, reason,
  spam score, SPF/DKIM/DMARC and who released or rejected it.
- Hold-queue workflow: `held triage` groups the queue by reason, policy or sender
  domain with a release-risk class and ready-to-act ID batches (`--new` shows only
  what arrived since the last run); `held release-safe` checks the hold reason
  before releasing and records the ticket in a local ledger; `held purge-sender`
  rejects every held message from one sender, optionally blocking it, and only
  plans until `--yes`; `held release-audit` flags risky releases with no ticket.
- Phishing response: `ttp who-clicked` lists who clicked a raw or
  Mimecast-rewritten URL; `threats mailbox` shows every threat event for one
  mailbox; `threats sender` lists every recipient one sender or domain reached,
  who clicked and what is still held.
- `mailflow`: one OK/WARN/FAIL verdict for queue backlog, inbound gaps and stuck
  deliveries, with a monitor-friendly exit code; `senders suggest` proposes narrow
  permits and blocks from cached release-log history.
- Every covered API 2.0 endpoint (hold queue, message tracking, TTP, DLP, audit,
  archive logs, gateway, journaling, managed senders, Cloud Integrated policies
  and remediation) as a typed command.
- Delegate access: `MIMECAST_ACCOUNT_CODE` or `--account-code` selects one
  customer account on the classic endpoints; REST endpoint writes are refused
  while it is set.

### Security
- Built on the shared credential kit: credentials are held from every source
  before any output, and every output path (terminal, MCP results, errors, local
  files, the local store) withholds a unit that carries a held value in any
  encoding the credential leak contract lists. An answer that carries a
  credential, such as a journaling connection's password, is withheld whole and
  never stored.
- A request or redirect that would carry a credential to any host other than the
  configured Mimecast API and token hosts is refused.
- Every MCP tool that changes Mimecast previews until called with
  `confirm: true`; `MIMECAST_MCP_READ_ONLY=1` refuses every write.
- A Mimecast HTTP 200 answer that carries a `fail[]` entry is reported as a
  failure, not a success.
