# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.2.2] - unreleased

### Fixed
- OAuth2 authentication now resolves the `{tenant}` and `{domain}` endpoint
  placeholders in the token URL, so `auth login`, automatic token minting, and
  refresh no longer fail with `invalid character "{" in host name`, on Windows
  or any platform. The token-mint paths previously POSTed the literal
  `https://{tenant}.{domain}/auth/token` because they bypassed the URL
  substitution used for ordinary requests. Thanks to @Xenith-B for the report
  (#147).

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.2.0] - unreleased

### Changed
- Maintenance and packaging updates.

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
