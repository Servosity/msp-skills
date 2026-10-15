# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-10-15

### Added
- Initial msp-skills release: QuoteWerks Web CLI (`quotewerks-cli`) and MCP server
  (`quotewerks-mcp`) over the QuoteWerks Web REST API (beta), authenticated with an
  `X-API-Key` (`QUOTEWERKS_API_KEY`).
- Typed quote commands that use the real QuoteWerks column names: `docs search`,
  `docs get` (every line, line types decoded, margin per line), `items list`,
  `products search` across every product database.
- Previewed writes that change nothing until `--confirm`: `docs create`, `items add`
  (filled from your product databases), `items reprice` (old versus new cost, price
  and margin per line) and `requote` (an earlier quote or order copied into a new
  quote at today's prices).
- `docs lint`: price below cost, missing cost, bad quantity and low-margin lines on a
  quote before it is sent; `--strict` exits 1 on errors.
- Local mirror (`mirror`) and the reports that read it: `margin`, `winrate`,
  `pipeline`, `cost-drift` and `changes`.
- Every vendor endpoint mirrored: `tables` (list, rows, row, query, schema, values,
  insert, update, delete), `cloud-settings`, `reports run`, `identity userinfo` and
  the QuoteWerks Web AI agent (`agent ask` and the report-drafting previews).
- MCP server refuses local file-path flags, and `import` is CLI-only.
