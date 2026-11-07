# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-11-06

### Security
- **Updated two compression libraries to fix GO-2026-6888 and GO-2026-6865.** `github.com/andybalholm/brotli` moves from v1.0.6 to v1.2.8: a crafted Brotli stream could make the program allocate excessive memory while decompressing it (GO-2026-6888). `github.com/klauspost/compress` moves from v1.18.7 to v1.20.2: a crafted zstd stream with two-block frames could make the program crash with a panic while decompressing it (GO-2026-6865). No other dependency changed and commands behave the same.

## [0.1.1] - 2026-11-05

### Fixed
- Output that holds no credential is no longer replaced by "withheld ... it carried a credential". Long or rich text, such as HTML email with Outlook Safe Links or long incident reports, ran out the shared credential kit's checking budget and was withheld as if it carried a credential. A credential the program holds is still withheld ([#571](https://github.com/Servosity/msp-skills/pull/571)).

## [0.1.0] - 2026-11-05

### Added
- Initial msp-skills release: Kaseya VSA 10 CLI (`kaseya-vsa-cli`) and MCP server
  (`kaseya-vsa-mcp`) covering the VSA 10 REST API: devices, assets, organizations,
  sites, groups, custom fields, notifications, webhooks, patch management, scripts,
  tasks, workflows and audit logs.
- Offline fleet questions from a local mirror: `patch-exposure` (pending OS updates
  ranked by KB, client or device, with CVSS and CVE filters), `health` (offline,
  antivirus, firewall, UAC, disk, CPU and memory checks), `software` (installs, version
  spread, devices below a version or missing an application) and `audit-logs digest`.
- `devices brief` for one device's current state, and `scripts run-wait` /
  `tasks run-wait` to run automation and collect exit codes and output in one call.

### Security
- The API token is sent only to the configured VSA 10 host; requests and redirects to
  any other host are refused.
- Custom-field values, webhook signing keys and the Windows product key are never
  written to the local mirror, and the on-disk response cache is off.
- Any output that repeats a held credential, in any encoding, is withheld whole by the
  shared credential kit: on the CLI, in MCP results, in files, in local store statements
  and in requests. A response carrying a webhook signing key is withheld the same way.
- MCP tools that change VSA 10, run automation or write local files return a preview and
  act only with `confirm: true`.
