# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Granola CLI + MCP server.
- Offline SQLite mirror of your meetings with FTS5 full-text `search` across
  notes, AI summaries, and transcripts.
- Cross-meeting queries: `attendee timeline` / `attendee brief`, `talktime`
  roll-ups, `stats` (attendees, duration, frequency), and `calendar overlay`
  missed-recording detection.
- `export` / `export-all` to combined notes + summary + transcript markdown.
- `sync` from the Granola public REST API with detail-endpoint enrichment;
  `doctor` health/auth check.
