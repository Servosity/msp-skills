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
- MCP `cisco-meraki_execute` reports `has_more` when Meraki has another page, with the
  `startingAfter` cursor when the whole page fit in the result (and, when it did not,
  tells the agent to shrink `perPage` rather than hand it a cursor that skips items),
  instead of returning page one as if it were the whole list. This covers both bare
  arrays and `{items: [...]}` envelopes, and a truncated last page.
- `sync` exits 0 on real orgs: rows with no id field (VPN and uplink statuses, packet
  loss and usage by device, network and SSID, sensor readings, wireless-controller
  views, webhook and API request logs) are keyed on the network, device, SSID or
  client they describe; `[{items, meta}]` responses are unwrapped into their records;
  and a generated date-detection pattern no longer misreads timestamp-plus-serial
  keys as dates. Before, each such resource dropped every row and `sync` exited 1.
- `fleet changes --data-source local` no longer says the mirror was never synced
  right after a sync.
- `doctor` reports a 404 from its health check as a warning that names
  `CISCO_MERAKI_BASE_URL`, instead of calling a wrong base URL "reachable". The default
  base URL is the literal `https://api.meraki.com/api/v1`.
- Fleet commands name each failed org on a `warning:` line, so MCP agents see which
  org failed, not just how many.
