# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: the Apple Business CLI (`apple-business-cli`) and MCP
  server (`apple-business-mcp`), printed on CLI Printing Press 4.34.2.
- Every Apple Business API read and write: devices, MDM servers, device activities,
  AppleCare coverage, Activation Lock, users, user groups, org units, apps, packages,
  Blueprints, Configurations and audit events.
- Fleet reports from a local store per customer org: `devices unassigned`,
  `devices locked`, `applecare expiring`, `fleet summary` (one row per org),
  `mdm-servers health`, plus `devices lookup` by serial, MAC, IMEI, EID or order number.
- `devices assign` and `devices unassign` move serials between MDM servers by name,
  with `--dry-run`, `--wait` and a per-serial verification.
- `audit-events digest` and `audit-events actors` turn a relative window into readable
  change logs and per-actor counts.
- ES256 client-assertion authentication; one API account per customer organization.
- Credentials are held from startup and withheld, in any encoding, from every output,
  file, store write and MCP result; they are sent only to Apple's hosts.
- Over MCP, every write tool previews and runs only with `confirm: true`.
