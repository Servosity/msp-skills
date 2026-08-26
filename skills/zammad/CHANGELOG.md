# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - 2026-08-26

### Fixed
- **`doctor` reported health it had not established.** It treated any HTTP response to `GET /` as
  a healthy API, so a base URL aimed at the vendor's web UI - where every API path 404s - rendered
  exactly like a working install.
  A credential probe that came back 404 was reported as `ok (... but auth was accepted)`, which is a
  claim the probe never supported - it had not reached anything that could check the credential.
  That case now reports the base URL as wrong instead of the connector as healthy.
  It also dialled the shipped placeholder base URL (`https://your-instance.zammad.com/api/v1`) and rendered the resulting failure
  as `FAIL API: unreachable`, telling an operator who had supplied every credential the install asked
  for that they were broken. It now refuses to dial a placeholder and names `ZAMMAD_BASE_URL`,
  the variable that actually fixes it.
  `--fail-on` no longer scans hints and file paths for the word "error", which is what made it trip on
  healthy connectors.

- **The install prompted for the wrong credentials.** The binary reads environment variables that the
  Claude Desktop bundle never declared, so you were asked for the wrong set and the connector could not
  authenticate. Now declared on every install channel: `ZAMMAD_BASE_URL`, `ZAMMAD_URL`.

## [0.1.1] - 2026-08-17

### Security

- Go toolchain bumped to **go1.26.6**, which fixes **GO-2026-6218** (quadratic
  complexity in `net/url`, reachable from `cliutil.ProbeReachable`). The
  previously released binary was built with go1.26.5 and carried the advisory.
  CI could not catch this: the workflows request `go-version: "1.26"`, which
  resolves to the latest patched Go, so the security gate scanned a patched
  toolchain while the build honoured the pinned one. See issue #210.

## [0.1.0]

### Added
- Initial msp-skills release: Zammad CLI + MCP server with an offline SQLite mirror.
- Full ticket surface: list, get, search (Zammad query syntax), create, update, delete, plus articles (read/add) and a one-line `ticket note`.
- Knowledge Base: browse, search, and read answers (parsed from the init bundle) plus create/publish/set-internal/delete.
- Team-management analytics the API can't answer in one call: `agent-load`, `agent-trend`, `customer-health`, `overdue`, `escalate`, `churn-risk`, and `feedback-scan`.
- Reference reads: organizations, users, groups, states, priorities, tags, overviews.
- Per-instance config (`ZAMMAD_URL` + `ZAMMAD_API_TOKEN`); works with any self-hosted or hosted Zammad.
