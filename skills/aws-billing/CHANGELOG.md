# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [4.19.0]

### Added
- Initial msp-skills release: AWS billing CLI + MCP server, read-only against AWS.
- Plain-English bill breakdowns: `bill`, `consolidated` (per linked-account rollup with
  month-over-month delta), `compare` (rank what moved), `forecast`, and `explain` (decode
  opaque usage-type strings).
- Dollar-ranked waste hunters: `waste rank`, `waste transfer`, `waste gp2-gp3`, plus idle
  EC2, unattached EBS, orphaned snapshots, and unassociated Elastic IPs.
- Local SQLite mirror via `sync` so follow-up questions are instant and free instead of
  $0.01 per Cost Explorer call.
- `iam-setup` mints a least-privilege read-only policy, CloudFormation template, or bootstrap
  script so you can share billing access without over-granting.
- `ask` answers plain-English questions from the cache; `report --post-slack` posts a summary
  to Slack on opt-in.
