# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
