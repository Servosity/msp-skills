# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-09-18

### Changed
### Added
- **`RIVERSIDE_FM_NO_CONFIG_WRITE=1` keeps credential material off disk.** The rotated
  session cookie stays in memory for the running command and is never written to
  `config.toml`; `auth login` refuses to persist under the switch; wipes still run. Declared
  in the bundle prompt, the MCP Registry entry and the manual-install JSON.

### Fixed
- **The MCP server resolves its config the way the CLI does.** It used a hard-coded
  `~/.config/riverside-fm-cli/config.toml` and ignored `--config` and the switch above.
- **Windows bundle could not find its companion CLI.** The sibling lookup now tries
  `riverside-fm-cli.exe` first on Windows, then the bare name, then `RIVERSIDE_FM_CLI_PATH`,
  then `PATH`.

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)

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
