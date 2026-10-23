# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.2] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

### Security
- **An error can no longer show a credential the server repeated back in an encoded form.** The CLI and the MCP server masked a key, token or password only as written or URL-escaped. When an API error or an HTML page returned in place of JSON echoed it HTML-entity-encoded, JSON-escaped, double URL-encoded, base64 or hex, it reached the terminal, `--json` and `--agent` output, and MCP tool results. Now any such text that still carries a credential this program holds, in any of those forms, is replaced whole by `[text withheld: it repeated a secret value this program holds]`; the HTTP status, the hint and the exit code are kept. This covers TD_SYNNEX_REFRESH_TOKEN from the environment too. The same check also runs on every error a command prints, including `--json` and `--agent` error output, and on every MCP error result; successful output is unchanged.

## [0.1.1] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.0] - 2026-10-20

### Added
- Initial msp-skills release: `td-synnex-cli` and `td-synnex-mcp` for the
  StreamOne ION v3 reseller API (customers, catalog, pricing, carts, orders,
  subscriptions, cloud profiles, reports), with a local SQLite store.
- Licence book: `subscriptions renewals`, `subscriptions changes`, `margin`,
  `orders stuck` and `customers show` answer whole-book questions offline.
- Cross-distributor true-up: `position export` writes licence-position/v1 rows and
  `position trueup` joins them with Pax8, Sherweb or PSA exports.
- `subscriptions set` builds the ION UPDATE order for seats, renewal setting, name,
  status or tier, and places it only with `--confirm`.
- `auth mint` / `auth check` handle ION's single-use refresh tokens: the rotated
  token is saved under a lock before anything prints.
- `TD_SYNNEX_MCP_READ_ONLY=1` removes every MCP tool that can change ION.
- Credentials ION returns in customer, order and provisioning data are blanked
  before anything is printed, cached, synced or handed to an agent.
- Not yet run against a live ION tenant; margin assumes ION prices per subscription
  (see the README's Known gaps).
