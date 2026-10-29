# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - unreleased

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
- Any output that repeats a held credential, in any encoding, is withheld on the CLI,
  in MCP results, and in `--deliver` and `export` files.
- MCP tools that change VSA 10 or run automation return a preview and act only with
  `confirm: true`.
