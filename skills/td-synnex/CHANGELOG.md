# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

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
