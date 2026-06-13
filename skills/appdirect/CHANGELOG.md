# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.1] - unreleased

### Changed
- Regenerated on the printing-press 4.24.0 engine: more reliable fleet sync, corrected pagination across large result sets, robust numeric-ID handling, and dependency security updates. Same commands and workflows, sturdier local mirror.

## [0.1.0]

### Added
- Initial msp-skills release: `appdirect-cli` and `appdirect-mcp`, covering the
  documented AppDirect marketplace REST surface - companies, users, memberships,
  subscriptions, billing, assisted sales, catalog, and checkout.
- Offline SQLite mirror (`sync`) with full-text `search` and `analytics`.
- Cross-company billing views: `reconcile` (active-but-unbilled, overdue,
  failed-payment), `payments unpaid`, and `subs changed`.
- Single-customer rollup `company show` and assisted-sales `pipeline` /
  `pipeline stale`.
- OAuth2 client_credentials auth with an invisible token mint/refresh and a
  white-label `APPDIRECT_BASE_URL` override.
