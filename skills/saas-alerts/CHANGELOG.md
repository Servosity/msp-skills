# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: SaaS Alerts CLI (`saas-alerts-cli`) and MCP server
  (`saas-alerts-mcp`) covering the Reports, Manage, Respond and Webhooks APIs with one
  partner API key. The Respond access token is minted from that key automatically.
- `triage`, `investigate`, `cross-tenant` and `coverage` answer across every customer
  from a local store, and say `synced: false` with a note naming the fix, instead of an
  empty or partial all-clear, when the data was never synced, the last events sync did not
  finish, or the window asked about starts before the oldest synced events.
- `rule-triggers queue` lists pending and failed Respond triggers by how soon they expire
  or auto-ignore; `rule-triggers decide` applies one decision to many triggers, previewed
  until `--confirm`.
- `contain` blocks sign-in, expires sessions and resets the password for one Microsoft
  365 account in order, previewed until `--confirm`.
- `rule-triggers list --all` follows Respond's cursor across every page, and Unify device
  syncs page with `searchAfter`.
- `subscriptions health` checks every webhook subscription, its unsent queue and domain,
  and plans re-enabling disabled ones (`--reenable --confirm`).
- `SAAS_ALERTS_MCP_READ_ONLY=1` limits the MCP server to reads and previews.

### Security
- Webhook subscription delivery tokens are withheld before they are printed, cached,
  stored or returned over MCP.
- Values from the API cannot drive the terminal: `--csv`, `--plain` and `--quiet` drop
  control characters, and JSON output escapes C1 controls.
- An environment `SAAS_ALERTS_BASE_URL` is never saved to `config.toml`.

### Known limitations
- Not yet run against a live SaaS Alerts tenant.
- A partially rejected sync is reported as a `sync_warning`, so `sync --strict` can exit
  0 when some pages were refused.
- Unify device lists page with `searchAfter` from `meta.sort`, as the Manage spec
  describes. If a tenant returns a bare list instead, more than 1,000 devices cannot be
  synced completely; sync and `devices list --all` report truncated, never complete.
