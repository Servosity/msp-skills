# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-04

### Added
- Initial msp-skills release: a Blackpoint Cyber CompassOne CLI (`blackpoint-cli`) and MCP
  server (`blackpoint-mcp`), built on CLI Printing Press 4.34.2.
- Every CompassOne endpoint: accounts, tenants, detections and their alerts, assets,
  vulnerabilities, scans, external exposures, dark-web exposures, posture, reports, contact
  groups, Cloud Response (Microsoft 365, Cisco Duo, Google) connections and approved countries.
- `fleet-sync`: pulls every tenant's detections, devices, vulnerabilities, scans, exposures,
  posture, contact groups and Cloud Response settings into a local mirror; failed or
  truncated fetches are recorded, never read as empty.
- Cross-tenant commands over that mirror: `triage`, `stale-agents`, `gaps`, `cve-spread`,
  `leaderboard`, `geo-audit` and `overview`; and `client-report`, a live one-call client
  reporting bundle.
- Reads that carry `x-tenant-id` always go to the live API, so one tenant's local rows are
  never served for another.
- Credentials are withheld from every output, error, log, cache and store write, in any
  encoding, by the shared credential kit; a dark-web exposure's password and a Duo secret
  key in API responses are replaced by `[withheld]` in every mode and never written to
  the cache or the local store.
- MCP writes return a preview until the call passes `confirm: true`;
  `BLACKPOINT_MCP_READ_ONLY=1` hides every write tool.
