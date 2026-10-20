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
- `rule-triggers list --all` follows Respond's cursor across every page; Unify device syncs
  and the `devices` list commands page with `searchAfter`, and the `devices` commands read
  live only (their local copy cannot answer per-account questions).
- A sync narrowed with `--param`, `--global-param` or `--resource-param` stores its rows but
  never prunes, never moves the incremental watermark or saves a resume cursor, and is
  reported as a warning (sync exits 1 when every resource it ran was narrowed). Parameters a resource requires do not
  count as narrowing, and overriding the `customerId` the users sync sets per customer is
  refused.
- A full sync prunes users removed upstream from a customer that is still synced; resources
  that need scoping parameters (`events-fileshare`, the mailbox-rule and Unify reports) are
  never pruned as a whole.
- An explicit incremental events sync re-reads the 6 hours before the previous explicit
  sync, so an event SaaS Alerts recorded late is still fetched.
- `subscriptions health` checks every webhook subscription, its unsent queue and domain,
  and plans re-enabling disabled ones (`--reenable --confirm`).
- `subscriptions test-event` exits 6 with `success: false` when SaaS Alerts answers
  `false`, an error message, a string it does not recognise as delivered, or a
  delivery-error list, instead of reporting the failed delivery as a success (the same
  rule `subscriptions health` uses); the MCP server returns the same answer as an error.
- `rule-triggers decide --ids` reads any requested id it does not find on the fetched
  pages directly, so `--max-pages` cannot leave one out; when the list was cut it says
  supersession was checked only against the pages it read (`supersession_unchecked`).
- `SAAS_ALERTS_MCP_READ_ONLY=1` limits the MCP server to reads and previews.

### Security
- Webhook subscription delivery tokens are withheld before they are printed, cached,
  stored or returned over MCP, including in a `token` field of an API error answer, and a
  token of 8 or more characters that the request sent is masked if an error answer
  quotes it back as written, URL-encoded, in an HTML error title, or JSON-escaped in a
  JSON answer.
- Tenant values cannot drive the terminal through command output: `--csv`, `--plain`,
  `--quiet` and the human output of the workflow commands drop control characters, and
  JSON output escapes C1 controls.
- A Webhooks API write whose token is the `[withheld]` placeholder is refused, so copying a
  listed subscription into an update never overwrites the real token.
- The partner API key and Respond access tokens are masked in error messages even when
  the API's answer JSON-escapes them or they fall at the point where long error text is
  cut.
- An environment `SAAS_ALERTS_BASE_URL` is never saved to `config.toml`.

### Known limitations
- Not yet run against a live SaaS Alerts tenant.
- A partially rejected sync is reported as a `sync_warning`, so `sync --strict` can exit
  0 when some pages were refused.
- Unify device lists page with `searchAfter` from `meta.sort`, as the Manage spec
  describes. If a tenant returns a bare list instead, more than 1,000 devices cannot be
  synced completely; sync and `devices list --all` report truncated, never complete.
