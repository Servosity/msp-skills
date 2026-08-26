# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## Unreleased

### Fixed
- **A test held a fixed 2026 alert in range with a ten-year look-back.** `customers profile`
  counts RMM alerts newer than the window you ask for, measured against the current time. The
  test seeded an alert dated `2026-06-05` and stretched `--alert-window` to `3650d` so it would
  still be counted - which buys ten years and no more. Around 2036 the alert ages out and the
  count silently drops to zero. The alert is now seeded an hour before the clock and the window
  is back to a realistic `7d`, matching the sibling test in the same file. No shipped behaviour
  changed - the command was correct throughout.

## [0.1.1] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://{subdomain}.syncromsp.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `SYNCRO_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `SYNCRO_BASE_URL`.

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `syncro-cli` + `syncro-mcp` for Syncro PSA and RMM.
- Local SQLite mirror with full-text `search` and `sync` for offline,
  cross-customer analysis that never touches your API rate limit.
- Billing-leakage analytics: `billing uninvoiced`, `billing drift`,
  `billing ar-aging`, and `customers margin`.
- Service-desk and RMM rollups: `tickets aging`, `assets patch-gaps`,
  `alerts noise`, `alerts orphans`, and the cross-entity `customers profile` card.
- Full PSA + RMM command surface (tickets, invoices, estimates, contracts,
  customers, assets, RMM alerts) with `--agent` JSON mode and `--dry-run`
  preview for every write.
