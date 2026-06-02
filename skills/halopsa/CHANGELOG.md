# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - 2026-06-02

### Fixed
- OAuth2 client-credentials tokens now send the `scope` parameter (defaulting to
  `all`), fixing HTTP 401 on every authenticated HaloPSA API call (refs #7).

### Changed
- First marketplace-ready release: one-click `.mcpb` install, validated plugin
  manifest, and registry metadata aligned for submission.

## [0.1.0] - 2026-05-26

### Added
- Initial msp-skills release: HaloPSA CLI (`halopsa-cli`) + MCP server
  (`halopsa-mcp`).
- Ticket triage, SLA-breach pre-emption, and cross-client analytics.
- Local SQLite mirror with full-text search for fast, offline cross-entity
  queries the live API can't return in one shot.
- Cross-agent install: Claude Desktop `.mcpb`, Claude Code / Codex / Cowork,
  GitHub Copilot, Gemini CLI, ChatGPT (remote), Microsoft 365 Copilot (remote),
  Hermes, and OpenClaw.
