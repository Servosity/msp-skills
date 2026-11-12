# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.5] - unreleased

### Fixed
- **`store_status` no longer stays `partial` for good when your token cannot read a resource.** A resource the vendor refuses this token (HTTP 403) is now recorded as skipped. `search` and `sql` report `complete_with_skips` when everything else finished and name what was skipped under `skipped_forbidden`. A resource that failed still leaves the store `partial` and is named under `incomplete_resources`, and the `sync` summary names both. The `sync` exit code is unchanged ([#692](https://github.com/Servosity/msp-skills/issues/692)).
- **`sync` progress reports the rate it measured.** `rate_rps` in the `sync_progress` event used to be the request pacer's ceiling. It now carries the measured rows per second over the last 30 seconds, the same value as the new `rows_per_sec`. The ceiling, when one is active, is the new `limiter_ceiling_rps`, and the progress line reads like `[33.0 rows/s, limit 4.9 req/s]` ([#694](https://github.com/Servosity/msp-skills/issues/694)).
- **`sync` spends far less CPU on progress output.** Every line the CLI prints goes through the credential check, and that check costs about the same whether the line is long or short. `sync` printed one progress line per page. It now prints the first page's line and then at most one every 5 seconds per resource; each line still carries the measured rate, and the final count still arrives in `sync_complete` and the done line ([#696](https://github.com/Servosity/msp-skills/issues/696)).

## [0.1.4] - 2026-11-11

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.3] - 2026-11-10

### Fixed
- **`sync` costs milliseconds of CPU per row, not about a second.** The credential check on rows written to the local store read every row through each decoder it knows, which pinned a CPU for hours on a large first sync. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread: a 1,177-record first sync went from 354 CPU-s to under 1. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).
- **A withheld row no longer fails `sync`.** `sync` names the withheld row IDs on stderr and in its summary (`"withheld"`), still exits 0 when every other row stored, and keeps that resource's checkpoint, so the next run fetches the row again (#612).
- **`sync` progress shows a real request rate.** On an API that sends no rate-limit headers, `rate_rps` in sync progress climbed without bound (6.6e24 was seen), because the request pacer kept raising a rate no request reached; after that a rate-limit answer could no longer slow requests down. The pacer now raises its rate only while it is holding requests back.

## [0.1.2] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release of the DNSFilter connector: `dnsfilter-cli` plus the
  `dnsfilter-mcp` MCP server, printed by cli-printing-press 4.34.2 from DNSFilter's
  published OpenAPI spec (every API operation), authenticated with a DNSFilter API
  token sent as the bare `Authorization` header value.
- `why-blocked`: the site or roaming-client policy (current scheduled-policy slot
  included) and the rule that blocks or allows a domain for one client org, with
  recent Query Log hits; says which rule types the policy API does not expose.
- `allow`: plans an allow on the policies that actually blocked the domain (Query Log
  attribution, else the assigned site or roaming-client policies that block it), refuses threat or
  unresolvable categories without `--force`, skips global and non-editable
  policies; `--confirm --note` writes, reads back and exits 6 on any failed target.
- `unblocks triage`: pending end-user unblock requests across every client, grouped
  by domain, with threat flags, age and DNSFilter's own already-covered check.
- `agents drift` and `agents stale`: roaming client version drift (against the live
  release feed, or a named bad build) and silent, unprotected or bypassed clients,
  grouped by client org, from the local mirror.
- `lists where` and `policy-audit`: every policy whose allow or block list holds a
  domain or a parent domain (with the sites and roaming clients it governs, directly
  or through any slot of a scheduled policy), and a hygiene sweep (global, MSP-shared
  policies included and flagged) for threat-categorized allow-list
  entries, unblocked security categories and drift from a baseline policy.
- Local SQLite mirror (`sync`, `search`, `analytics`). Secret fields DNSFilter returns
  (site secret keys, API key tokens) are never written to it or to the response cache
  and read `[withheld: credential]` in every answer; read a secret in the DNSFilter
  dashboard. `user-agents uninstall-pin` and `psa-integrations` are refused: the whole
  answer to each is a credential.
  and `api-keys` is not synced by default. A collection reads as synced only after a
  complete walk.
- MCP: reads go through `dnsfilter_read`; `dnsfilter_execute` previews every
  state-changing call and every credential read, and sends only with `confirm: true`.
  `DNSFILTER_MCP_READ_ONLY=1` (or `true` / `yes`; it fails closed) removes `allow`,
  `sync`, `export`, the archive tool and `dnsfilter_execute` and refuses API key reads
  and site secret-key endpoints. Writes through `dnsfilter_execute` refuse
  parameters outside the endpoint's schema; `import` and file-path flags are not
  offered over MCP. HTTP mode needs a bearer token of at least 32 characters.
- A shared credential kit holds the token and every value in the config and credentials
  files before anything prints, and withholds any output, file, database write, child
  process or off-origin request that carries one.
- Redirects never leave the API's origin (another scheme, host or port is refused) and
  never carry the API key or any configured header; `doctor` reports a redirected
  probe instead of trusting it.
- `sync` follows DNSFilter's `meta.total_pages` paging, so a short first page no longer
  ends a walk early.
- Scheduled policies resolve on Windows without Go installed (IANA time zone data is
  embedded).
- Safety: saved profiles never supply `--confirm`, `--force` or `--yes`; `--dry-run`
  covers local mutations too (export files, feedback, auth, profile delete); `--deliver`
  captures every command's output; tenant strings are escaped before they reach a
  terminal or an MCP client; `doctor` checks the token against `/v1/current_user` and
  warns when a stored key starts with `Bearer ` or `Token `; Windows (PowerShell) forms
  sit beside every POSIX auth hint.
