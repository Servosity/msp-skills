# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.3] - unreleased

### Changed
- Internal: two tests of the learning journal's lock handling no longer depend on how fast the machine is (they failed on slow Windows CI runners). The connector itself behaves exactly as before.

## [Unreleased]

### Changed
- `cli/NOTICE` now names the product `cisco-meraki-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

### Fixed
- `--json` turns on JSON output on `networks firmware-upgrades update-network-staged-stages` again: the command declared its own `--json` value flag for its body. The JSON array of staged upgrade groups is now `--stages-json`.

## [0.1.2] - 2026-10-21

### Fixed
- **Destructive commands no longer answer to `create`.** The generator had given these commands the hidden alias `create`, so typing `create` in their place ran a destructive or disruptive request (a delete, cancel, refund, stop, reboot, install or script run, for example) instead of creating anything. For example `cisco-meraki-cli devices reboot create` ran `cisco-meraki-cli devices reboot device`, POST `/devices/{serial}/reboot`. The alias is removed; each command still runs under its own name. If a script reached one of these through `create`, call it by the name below:
  - `cisco-meraki-cli devices appliance create-device-interfaces-ports-update` (POST `/devices/{serial}/appliance/interfaces/ports/update`; sets appliance port state, including enabled=false, which takes the port down)
  - `cisco-meraki-cli devices cellular create-device-uplinks-bands-masks-update` (POST `/devices/{serial}/cellular/uplinks/bands/masks/update`; sets which cellular bands the uplink may use; a narrow mask drops the uplink)
  - `cisco-meraki-cli devices reboot device` (POST `/devices/{serial}/reboot`)
  - `cisco-meraki-cli devices sensor create-device-command` (POST `/devices/{serial}/sensor/commands`; sends a command to a sensor (downstream power off or cycle, for example))
  - `cisco-meraki-cli networks bind network` (POST `/networks/{networkId}/bind`; binds a network to a template, replacing its configuration)
  - `cisco-meraki-cli networks firmware-upgrades create-network-rollback` (POST `/networks/{networkId}/firmwareUpgrades/rollbacks`)
  - `cisco-meraki-cli networks pii create-network-request` (POST `/networks/{networkId}/pii/requests`; submits a request to delete or restrict processing of a client's personal data)
  - `cisco-meraki-cli networks split network` (POST `/networks/{networkId}/split`)
  - `cisco-meraki-cli networks unbind network` (POST `/networks/{networkId}/unbind`)
  - `cisco-meraki-cli organizations action-batches create-organization-action-batch` (POST `/organizations/{organizationId}/actionBatches`; creates and, with confirmed=true, runs a batch of arbitrary actions, destroy included)
  - `cisco-meraki-cli organizations campus-gateway batch-organization-clusters-tunneling-update` (POST `/organizations/{organizationId}/campusGateway/clusters/tunneling/batchUpdate`; rewrites campus-gateway tunnel settings for many networks at once)
  - `cisco-meraki-cli organizations integrations disable-organization-xdr-networks` (POST `/organizations/{organizationId}/integrations/xdr/networks/disable`)
  - `cisco-meraki-cli organizations licensing move-organization-coterm-licenses` (POST `/organizations/{organizationId}/licensing/coterm/licenses/move`)
  - `cisco-meraki-cli organizations spaces remove-organization-integration` (POST `/organizations/{organizationId}/spaces/integration/remove`)
  - `cisco-meraki-cli organizations switch clone-organization-devices` (POST `/organizations/{organizationId}/switch/devices/clone`; overwrites the target switches' port and switch settings)

### Security
- **Rebuilt with Go 1.26.10 (was 1.26.9).** Go 1.26.10 fixes GO-2026-6809 in `crypto/x509` (it distrusts compromised intermediate certificates), which the security gate reports for every connector still built on Go 1.26.9. No other dependency moved.

## [0.1.1] - 2026-10-15

### Fixed
- **Windows: the built-in Administrator account can save credentials.** On the built-in
  local Administrator (common on MSP servers and jump boxes), saving credentials failed
  with `token file DACL grants access to a non-owner principal (LA)`, and reads refused
  the saved file. The token-file permission check compared security-descriptor text
  instead of account SIDs; it now resolves each trustee to a SID first. The allowed
  principals are unchanged: your account, BUILTIN\Administrators and SYSTEM.
- **Windows: a token file with a NULL DACL is now refused.** A NULL DACL (no access
  control at all, so any account on the machine can read the file) written with its
  flags first, which is how Windows Server 2025 renders it, passed the check. The check
  now parses the DACL exactly and refuses anything it cannot parse.

## [0.1.0] - 2026-10-15

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
  and `fleet changes --data-source local`. `search` reads the mirror by default:
  Meraki's only search endpoint is per organization, so the generated live lookup
  failed every time; `fleet find` is the live lookup.
- MCP recipe tool `locate_a_mac_from_a_soc_alert` refuses flag-like values, so a tool
  call cannot smuggle a root flag such as `--deliver` into the CLI.
- MCP `cisco-meraki_execute` reports `has_more` when Meraki has another page, with the
  `startingAfter` cursor when the whole page fit in the result (and, when it did not,
  tells the agent to shrink `perPage` rather than hand it a cursor that skips items),
  instead of returning page one as if it were the whole list. This covers both bare
  arrays and `{items: [...]}` envelopes, and a truncated last page.
- `sync` exits 0 on real orgs: rows with no id field (VPN and uplink statuses, packet
  loss and usage by device, network and SSID, sensor readings, wireless-controller
  views, API request logs) are keyed on the network, device, SSID or
  client they describe; `[{items, meta}]` responses are unwrapped into their records;
  and a generated date-detection pattern no longer misreads timestamp-plus-serial
  keys as dates. Before, each such resource dropped every row and `sync` exited 1.
- Local-mirror rows written by `sync` are keyed on serials and real IDs, not display
  names, so two switches both called "Core Switch", or "Data" VLANs in several
  networks, stay separate rows instead of silently merging. Intrusion rules are keyed on policy plus
  rule ID (not the Snort generator ID shared by nearly every rule), group-policy
  assignments on the assignment ID (not the policy they point at), MX port profile
  assignments on device plus port, and a renamed network or access point keeps its
  row instead of leaving a stale copy behind. Configuration changes made by Meraki
  support or automation (no admin) keep their page and label in the key, so two such
  changes in the same second no longer merge. MX warm-spare settings by network are
  stored whole, including networks with warm spare disabled.
- `export organizations` follows the `Link` header, so a key with more than 100
  organizations exports every one once (it used to re-request page one), and the
  MCP `cisco-meraki_execute` count for `[{items, meta}]` pages is the number of records.
- A `sync` of only dependent resources exits 0 when it stored rows, even if one
  organization has API access disabled; each failed organization is still reported.
  `sync --strict` exits non-zero when any resource failed or completed with warnings,
  including one that stopped before its last page (`--max-pages`, or an API that
  repeated a page or cursor).
- `fleet push --apply` exits 6 when a target network's change did not land: its
  action batch was rejected, failed, or is still pending after `--wait`, or its
  current settings could not be read, or an organization's network list could not be
  read (an org with API access disabled is reported as a warning, not counted). The
  error gives the true count and names the first 20 targets and why. A blank `--org " "`
  no longer counts as a target, so it cannot slip past the untargeted-push refusal.
- The MCP server reports its release version in `serverInfo` (it was fixed at
  `0.0.0-dev`).
- `fleet changes --data-source local` no longer says the mirror was never synced
  right after a sync.
- Write commands no longer store their response in the local mirror. A routine SSID
  update stored the PSK and RADIUS secret it echoes, a webhook payload template its
  `Authorization` header, and an SSID or VLAN from two networks merged into one row
  keyed on its name or number. Organization-level data a write changes is refreshed
  by the next `sync`; network and device settings such as SSIDs and VLANs are not in
  the mirror at all.
- The local mirror redacts credentials by field name and change-log label. The
  synced configuration change log records an API change's request body, and `search`,
  `analytics` and the MCP `sql` tool could return it. Every credential field (`psk`, `passphrase`, `password`, `secret`,
  `sharedSecret`, SNMP community strings and v3 passwords, API keys and tokens, and the
  value of a header pair named like a credential, such as `Authorization` or
  `x-functions-key`, and the base64 webhook `headersFile` and `bodyFile`) is now
  stored as `[redacted]`. Key names match with `_`, `-` and case ignored, and any key
  ending in `token` or `key` counts (`api_key`, `access_token`, a PagerDuty
  `routing_key`), except `licenseKey`. A change-log value that is cut-off JSON is
  withheld whole as `[redacted: cut-off JSON]`, a webhook change's old and new values
  (free-form payload templates, HTTP server URLs) are withheld whole, as is any change
  value that mentions a webhook and every string under a `callback` object, webhook
  HTTP server IDs (the base64 of the receiver URL) and `url=` query values in
  change-log labels and API request paths are replaced with `[redacted:<short hash>]`
  in the mirror, in row IDs and in `fleet changes` and `fleet blame`
  output, `sync` no longer mirrors webhook delivery logs (each row carries the receiver
  URL), and a credential is never used as a row ID (a newly generated
  API key used to become one). Change-log old and new values, which carry an API
  change's whole request body, are redacted in the mirror and in `fleet changes` and
  `fleet blame` output. License keys are kept.
- A `fleet push` preview prints current credential fields (`psk`, a RADIUS `secret`,
  `communityString`), and every current string under a webhook path, as `[redacted]`
  instead of the live value.
- An MCP call can switch a default-on flag off: an explicit `false` is passed as
  `--flag=false` (the MCP `fleet_renewals` tool's `include-expired` used to ignore it).
- The MCP server refuses local-file flags (`--notes-file`, `--playbook-file`,
  `--playbook-notes-file`, `--input`), and the MCP `fleet_push` tool takes `body` only
  as inline JSON, so a tool call cannot make the server read a file of its choosing.
- Single-record GET commands print the whole record. Commands such as
  `organizations action-batches get-organization-action-batch` and
  `organizations snmp get-organization` used to print only one array field, hiding a
  batch's `status.failed` and errors.
- `networks devices vmx-network-claim` is treated as the write it is: it is no longer
  labelled read-only and is never resent after an ambiguous network error (it could
  claim a vMX up to four times).
- `fleet changes` judges `--max-age` by the change log's own sync time once a sync has
  walked it completely.
- `fleet fanout` counts a network with warm spare disabled as one record, matching
  `sync`.
- `doctor` reports a 404 from its health check as a warning that names
  `CISCO_MERAKI_BASE_URL`, instead of calling a wrong base URL "reachable". The default
  base URL is the literal `https://api.meraki.com/api/v1`.
- Fleet commands name each failed org on a `warning:` line, so MCP agents see which
  org failed, not just how many.
