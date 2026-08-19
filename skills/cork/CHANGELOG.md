# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release of the Cork connector: `cork-cli` and the `cork-mcp`
  MCP server, covering the full Cork REST surface (clients, devices, domains,
  inboxes, score history, compliance events and event types, vulnerabilities,
  software packages, integrations and their tenants/devices/users/credentials,
  warranties, invoices and line items, distributor, and the authenticated user).
- Offline SQLite mirror with `sync` and full-text `search`, so cross-client
  questions answer from local data instead of an O(clients) API fan-out.
- Eight commands the Cork API cannot answer in one call:
  - `score attribute` differences the claims, compliance, coverage, and
    vulnerability components of a client's risk score across a window and ranks
    which one drove the move.
  - `score regressions` ranks the whole book of business by score delta.
  - `vulnerabilities triage` orders software products by exploitability
    (known-exploited first, then EPSS, then CVSS) with a blast-radius device
    count. The API's server-side `sort_by` accepts only `sw_vendor` and
    `sw_product`, so this ordering is impossible upstream.
  - `vulnerabilities exposure` answers "are we exposed to CVE-X" by scanning for
    a CVE id that exists only nested inside `cves[]` with no endpoint filter.
  - `compliance overdue` joins compliance events against the event-type catalog
    to find events past their remediation window. The two halves live on
    different endpoints and nothing joins them.
  - `integrations health` catches the connector reporting `connection_status: ok`
    while its `last_synced_at` has gone stale.
  - `coverage gaps` diffs connector-reported devices against client-attributed
    devices on `associated_endpoints[].integration_identifier`.
  - `warranties exposure` ranks unwarranted and lapsed clients by current risk.
- `--agent` mode (JSON, non-interactive), `--dry-run` previews on every mutating
  command, typed exit codes, and `export`/`import` for JSONL backup and migration.
