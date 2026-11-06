# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: Device Warranty CLI (`warranty-cli`) and MCP server
  (`warranty-mcp`) for Dell TechDirect, Lenovo eSupport and Apple Business warranty data.
- `lookup` for one or many serials (or a CSV/JSON file on the CLI), vendor detected from the
  serial or named, normalized to one coverage-line shape and saved to a local warranty mirror.
- `import` of RMM or asset exports per client, `apple-pull` for a client's Apple Business devices.
- Offline fleet answers from the mirror: `report` (expired and expiring per client),
  `lifecycle` (coverage endings by quarter, devices past age), `unresolved` (what could not be
  verified, with one reason each), `recheck` (refresh stale and soon-expiring devices within
  each vendor's batch size and pace).
- Coverage events (`events list`, read-only MCP `events_list`) for coverage expiring, expired
  and changed, read with a cursor.
- Every vendor is optional: `doctor --json` names exactly what each vendor still needs, and
  proves a Dell credential with one read of Dell's entitlement endpoint.
