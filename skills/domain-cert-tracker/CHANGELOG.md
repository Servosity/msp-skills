# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **`sync` memory no longer climbs.** The credential check on every row a sync stores no longer grows with each page or spikes on one large record: between records memory falls back to where it started, so a first sync of a large account finishes in a 1 GB container. The check is exactly as strict as before (#612).
- **One withheld row no longer stops a sync.** A row the credential check keeps out of the local store is skipped instead of failing its whole batch: every other row is stored, `sync` names the withheld row IDs and exits non-zero, and that resource's checkpoint stays put, so the next run fetches the row again (#612).
- **`sync --concurrency` above 1 no longer interleaves writes.** Its parallel workers shared one event writer that nothing serialized, so `--deliver`, `--receipt-file` and `--audit-dir` could crash or record a receipt hash that did not match the output. Writes are now serialized (#676).

## [0.1.0] - 2026-11-06

### Added
- Initial msp-skills release of the Domain and Certificate Tracker CLI and MCP
  server, built on cli-printing-press 4.34.2. No account or credential needed.
- Client inventory (`clients set`, `clients import`, `clients list`,
  `clients remove`) kept in a local SQLite store. `clients set` and
  `clients remove` preview and exit 2 until `--yes` (or MCP `confirm: true`).
- `sweep`: checks every inventoried domain and host over RDAP, Certificate
  Transparency (crt.sh), live TLS and DNS, stores each result and raises typed
  change events (`domain.expiring`, `cert.expiring`, `ct.new_cert`,
  `dns.changed`, `registrar.unlocked`). A failed check is listed as unverified
  and never replaces the last good result.
- `report`, `renewals`, `issuers audit` (CAA judged per name, RFC 8659) and
  `hosts discover` answer from the local store with no network call.
- `issuers audit` judges each certificate's own SAN list, which `sweep`
  downloads from crt.sh (at most 50 per sweep, once each); a certificate whose
  full name list is not known yet is `names_partial`, never routine. A CAA
  set with an issuer-critical property the audit does not interpret makes the
  certificate `caa_unverified`.
- `renewals` measures a stalled deploy from the earliest newer same-CA
  certificate this tool saw, and matches the served certificate to CT by
  issuer, serial and validity.
- `events list`: reads change events after a cursor, once each.
- Single lookups: `domain`, `nameserver`, `ip`, `autnum`, `bootstrap`,
  `certificates`, `tls probe` and `dns check`.
- Outbound guard: HTTP lookups and TLS probes connect only to public
  addresses, on port 443 unless the command line passes `--allow-port`; MCP
  calls cannot open other ports or read a file from the host.
