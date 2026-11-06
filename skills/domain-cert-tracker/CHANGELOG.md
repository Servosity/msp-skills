# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
