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
  roll-ups, and `stats` (attendees, duration, frequency).
- `export` / `export-all` to combined notes + summary + transcript markdown.
- The MEMO pipeline: `preflight`, `extract`, `memo run`, and `memo queue`, with a
  transcript gate that reads the public API's `microphone` / `speaker` labels and a
  duplicate check keyed on meeting identity, not on a shared title.
- `sync` from the Granola public REST API with detail-endpoint enrichment, and
  optional private notes through Granola's official MCP (`mcp-auth login`).
- `doctor` verifies the API key with a real read and says plainly when the base
  URL is not the Granola API.
- Read-only against Granola: no command creates, edits, or deletes Granola data.
