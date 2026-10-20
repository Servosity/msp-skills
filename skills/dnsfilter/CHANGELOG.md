# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release of the DNSFilter connector: `dnsfilter-cli` plus the
  `dnsfilter-mcp` MCP server, printed by cli-printing-press 4.33.5 from DNSFilter's
  published OpenAPI spec (every API operation), authenticated with a DNSFilter API
  token sent as the bare `Authorization` header value.
- `why-blocked`: the site or roaming-client policy (current scheduled-policy slot
  included) and the rule that blocks or allows a domain for one client org, with
  recent Query Log hits; says which rule types the policy API does not expose.
- `allow`: plans an allow on the policies that actually blocked the domain (Query Log
  attribution, else the assigned site or roaming-client policies that block it), refuses threat or
  unresolvable categories without `--force`, skips global and non-editable
  policies; `--confirm --note` writes, reads back and exits 6 on any failed target.
- `unblocks triage`: pending end-user unblock requests across every client, grouped
  by domain, with threat flags, age and DNSFilter's own already-covered check.
- `agents drift` and `agents stale`: roaming client version drift (against the live
  release feed, or a named bad build) and silent, unprotected or bypassed clients,
  grouped by client org, from the local mirror.
- `lists where` and `policy-audit`: every policy whose allow or block list holds a
  domain or a parent domain (with the sites and roaming clients it governs, directly
  or through any slot of a scheduled policy), and a hygiene sweep (global, MSP-shared
  policies included and flagged) for threat-categorized allow-list
  entries, unblocked security categories and drift from a baseline policy.
- Local SQLite mirror (`sync`, `search`, `analytics`). Secret fields DNSFilter returns
  (site secret keys, API key tokens) are never written to it or to the response cache,
  and `api-keys` is not synced by default. A collection reads as synced only after a
  complete walk.
- MCP: `DNSFILTER_MCP_READ_ONLY=1` (or `true` / `yes`; it fails closed) removes `allow`,
  `sync`, `export` and the archive tool, limits `dnsfilter_execute` to GET endpoints,
  refuses API key reads, site secret-key endpoints and the roaming client uninstall PIN,
  and withholds `secret_key` from networks reads. Writes through `dnsfilter_execute`
  refuse parameters outside the endpoint's schema; `import` and file-path flags are not
  offered over MCP.
- Scheduled policies resolve on Windows without Go installed (IANA time zone data is
  embedded).
- Safety: saved profiles never supply `--confirm`, `--force` or `--yes`; `--dry-run`
  covers local mutations too (export files, feedback, auth, profile delete); `--deliver`
  captures every command's output; tenant strings are escaped before they reach a
  terminal or an MCP client; `doctor` checks the token against `/v1/current_user` and
  warns when a stored key starts with `Bearer ` or `Token `; Windows (PowerShell) forms
  sit beside every POSIX auth hint.
