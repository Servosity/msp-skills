# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0] - 2026-06-06

### Added
- Initial msp-skills release: `levelio-cli` + `levelio-mcp` for the Level RMM API.
- Full coverage of devices, groups, tags, custom fields, custom-field values,
  alerts, and OS updates - list/show plus the write surface (create, update,
  delete, `alerts resolve`, `import`, and the `automations` webhook trigger).
- Offline SQLite mirror (`sync`) with full-text `search`, `--agent` JSON output,
  `--dry-run`, and `--data-source auto|live|local`.
- Cross-entity rollups no single Level API call returns: `at-risk`,
  `patch-posture`, `fleet`, `alert-triage`, `stale`, `client-scorecard`,
  `cf-coverage`, `security-posture`, `group-tree`, `since`, `alert-recurrence`,
  `reboot-due`, and `tag-audit`.
