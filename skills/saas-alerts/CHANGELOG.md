# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.
- `cli/NOTICE` now names the product `saas-alerts-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.2] - 2026-10-22

### Security
- The MCP server no longer passes `import --input` to the CLI, so a tool call cannot point the import at a file on the machine that runs the server.

## [0.1.1] - 2026-10-21

### Security
- **MCP tool calls can no longer read local files through `teach`.** Over MCP, `teach --playbook-file` / `--playbook-notes-file` and `teach-playbook --playbook-file` / `--notes-file` copied any file the MCP server's account could read into the local learning store, where the `playbook_list` and `recall` tools handed it back. The MCP server now leaves these four parameters out of the tool schemas and refuses a call that names them. Pass the content inline instead (`playbook-json`, `playbook-notes`, `notes`). The CLI is unchanged: run from your own shell, these flags still read your own files.
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

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
  365 account in order, previewed until `--confirm`. Unless a step provably never
  reached Respond or was refused (4xx), its outcome is reported as unknown and no
  further step is sent. `rule-triggers decide` and `subscriptions health --reenable`
  check such an outcome by re-reading instead of reporting a failure.
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
  stored or returned over MCP, including in a `token` field of an API error answer. A
  text field that quotes a token is replaced by `[withheld]`, and a Webhooks answer in
  which a token cannot be isolated to one field is withheld whole (the command prints
  `"[withheld]"`).
- Tenant values cannot drive the terminal through command output: `--csv`, `--plain`,
  `--quiet` and the human output of the workflow commands drop control characters, and
  JSON output escapes C1 controls.
- A Webhooks API write whose token is the `[withheld]` placeholder is refused, so copying a
  listed subscription into an update never overwrites the real token.
- When an error answer quotes back a credential this CLI sent (the partner API key, a
  Respond access token, or a webhook token of 8 or more characters), as written or
  JSON-escaped, URL-encoded or as HTML entities, the answer's text is withheld from the
  error message instead of being printed or returned over MCP. In a successful answer,
  any field that quotes one back is replaced by `[withheld]` (or the whole answer is
  withheld when the echo is not in a single field) before it is printed, cached, stored
  or returned over MCP. A base64, case-changed or partial echo is not detected, nor is
  a webhook token written unquoted in free text (`token=abc`).
- An environment `SAAS_ALERTS_BASE_URL` is never saved to `config.toml`.

### Known limitations
- Not yet run against a live SaaS Alerts tenant.
- A partially rejected sync is reported as a `sync_warning`, so `sync --strict` can exit
  0 when some pages were refused.
- Unify device lists page with `searchAfter` from `meta.sort`, as the Manage spec
  describes. If a tenant returns a bare list instead, more than 1,000 devices cannot be
  synced completely; sync and `devices list --all` report truncated, never complete.
