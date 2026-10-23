# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0] - 2026-10-19

### Added
- Initial msp-skills release: CloudAlly CLI (`cloudally-cli`) and MCP server
  (`cloudally-mcp`) over CloudAlly's Management API v2: distributor, reseller and
  customer accounts, backup tasks and their entities, backup status feeds, users,
  billing and recovery jobs.
- `auth login` signs in as one company account; `auth login --partner` signs in to
  the partner portal and reaches every customer. Both send the `client-id` and
  `client-secret` headers on every request and refresh the token on their own.
- `auth use-account <accountId>` mints an account-scoped token for one customer
  (`--reseller` for reseller partners) and `auth use-account --partner` switches back.
- `doctor` verifies the stored token with one authenticated read (`GET /v2/user`)
  and turns the API row red when `base_url` is not the CloudAlly API.
