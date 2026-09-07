# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Riverside CLI + MCP server for exporting your own
  Riverside.com account.
- `bulk export` - archive a whole studio's transcripts, assets, and HLS manifests
  to disk with a resume cursor.
- `grab` - priority-fallback download of a recording (transcript, then audio,
  then HLS video).
- `transcripts convert` / `transcripts talktime` - convert transcripts to
  VTT/SRT/TXT/JSON/Markdown and compute per-speaker talktime.
- `search` - SQLite FTS5 full-text search across the local mirror of your synced
  transcripts, projects, and recordings.
- `clips harvest` and `media refresh` - pull Magic Clips and refresh short-lived
  CloudFront signed URLs before they expire.
- `ready` / `wait` - check or block on take readiness (backup, transcription, AI).
- Cookie-session auth (`auth login --chrome`) for Pro / Live / Webinar accounts
  that can't issue a Business-plan API key.
