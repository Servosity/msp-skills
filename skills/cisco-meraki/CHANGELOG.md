# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.0]

### Added
- Initial msp-skills release: Cisco Meraki CLI + MCP server covering all 1,035
  Dashboard API operations as typed commands.
- Cross-org `fleet` views over every organization the API key can see: `fleet health`
  (worst-first per org), `fleet down` (offline/alerting devices and failed MX uplinks
  with downtime), `fleet renewals` (co-term, per-device and subscription licenses plus
  end-of-sale/support hardware), `fleet changes` (one change log for every org),
  `fleet find` (MAC, IP, serial or name to org and network), `fleet admins` (2FA,
  API-key and stale-access audit, offboarding view), `fleet flaps`, `fleet blame`
  (config changes just before each outage), `fleet fanout` (any org-scoped GET across
  orgs, with completeness per org), and `fleet push` (one settings change to many
  networks via action batches, preview by default, sends only with `--apply`).
- Meraki `Link`-header pagination for `sync`, `--all` and every fleet command; array
  query parameters sent in Meraki's `name[]=value` form so filters are honored.
- `sync` mirrors org-level data into local SQLite for offline `search`, `fleet find`
  and `fleet changes --data-source local`.
- MCP recipe tool `locate_a_mac_from_a_soc_alert` refuses flag-like values, so a tool
  call cannot smuggle a root flag such as `--deliver` into the CLI.
- MCP `cisco-meraki_execute` reports `has_more` and the `startingAfter` cursor when
  Meraki has another page, instead of returning page one as if it were the whole list.
- `doctor` reports a 404 from its health check as a warning that names
  `CISCO_MERAKI_BASE_URL`, instead of calling a wrong base URL "reachable". The default
  base URL is the literal `https://api.meraki.com/api/v1`.
- Fleet commands name each failed org on a `warning:` line, so MCP agents see which
  org failed, not just how many.
