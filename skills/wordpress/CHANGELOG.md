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

## [0.0.0]

### Added
- Initial msp-skills release: `wordpress-cli` plus the `wordpress-mcp` MCP server.
- Pages, posts, media, categories, tags, users, and settings over the WordPress REST API.
- `media upload` for binary multipart uploads (images, video, audio, PDF), returning the media id.
- Local SQLite mirror via `workflow archive` / `sync` with full-text `search` for offline cross-content queries.
- Agent surface: `--agent`, `--dry-run`, `--select`, named profiles, and `--deliver` output sinks.
