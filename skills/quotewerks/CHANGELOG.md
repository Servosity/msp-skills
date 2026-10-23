# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - 2026-10-23

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `quotewerks-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.2] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.1] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

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
