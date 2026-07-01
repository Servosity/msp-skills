# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.0.0]

### Added
- Initial msp-skills release: Maxio Advanced Billing CLI + MCP server.
- Offline revenue intelligence computed from a local SQLite mirror that snapshots
  each sync into a time series: MRR movement waterfall (new / expansion /
  contraction / churn / reactivation), net and gross revenue retention, quick
  ratio, and signup-month cohort curves.
- Per-customer recurring-revenue history (`mrr client`), new-logo tracking
  (`new-customers`), renewal exposure (`renewals`), usage-driven expansion vs
  contraction (`usage-drivers`), and normalized-vs-invoiced reconciliation
  (`reconcile`).
- Account triage rollup, full Maxio Advanced Billing endpoint surface, full-text
  search over synced data, and `--agent` JSON mode for AI agents.
