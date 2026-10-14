# cisco-meraki skill - governance and safety model

> Unofficial. Community-built skill for the Cisco Meraki Dashboard API. Not affiliated
> with, endorsed by, or sponsored by Cisco Systems, Inc.
> This page tells an MSP owner exactly what the cisco-meraki skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cisco-meraki-cli` binary (and `cisco-meraki-mcp`) against the
Meraki Dashboard API with one credential:

- `MERAKI_DASHBOARD_API_KEY` - a Dashboard API key (Dashboard > My profile > API access).
  It carries the permissions of the Dashboard admin account that generated it, in
  **every organization that admin can see**. A key from a full org admin can change
  anything in every client org; a key from a read-only admin can only read.

Optional:

- `CISCO_MERAKI_BASE_URL` - only for a regional dashboard
  (`https://api.meraki.ca/api/v1` Canada, `https://api.meraki.cn/api/v1` China,
  `https://api.meraki.in/api/v1` India, `https://api.gov-meraki.com/api/v1` FedRAMP).
  Unset means the global `https://api.meraki.com/api/v1`.
- `CISCO_MERAKI_MCP_HTTP_TOKEN` - required only to run `cisco-meraki-mcp --transport http`;
  callers must send it as a bearer token.
- `CISCO_MERAKI_MCP_READ_ONLY` - set to `1` to make the MCP server refuse every write
  (see "How to lock it down").

The key is read from the environment, or from `credentials.toml` under the CLI's data
directory when you save it with `auth set-token` (`auth logout` removes it). It
is never logged and never sent anywhere except the Meraki Dashboard API. Each org must
also have Dashboard API access enabled (Organization > Settings); an org without it is
reported as `reachable: false` by `fleet health`, not silently skipped.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you
  pass `--dry-run` first to preview the request without sending. There is no
  confirmation prompt. Make your agent's policy: preview, show the exact command, get
  approval, then run the write.
- **`fleet push` previews by default.** It reads each target network's current
  settings and shows a per-network diff; nothing is sent unless you add `--apply`.
  Untargeted pushes are refused - you must name `--org`, `--network`, `--network-tag`,
  `--product-type`, or pass `--all-networks`. With `--apply` it exits 6 when a target
  network's change did not land (batch rejected, failed or still pending after
  `--wait`, or its current settings unreadable) or an organization's network list
  could not be read, and names each one (the first 20, with the true count).
- **`fleet fanout` is GET-only.** It runs one org-scoped read against every org and
  cannot send a write.
- **Read commands change nothing in the Dashboard** - with the secret-bearing
  exceptions in the Credential tier below.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.
- **The MCP server blocks output-routing flags.** Tool calls cannot set `--deliver`,
  `--config`, `--home`, `--profile`, `--yes` or similar root flags, and a tool-call
  value cannot be smuggled in as an extra flag.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below that line. The tiers follow the HTTP method of
each command (`agent-context` lists it per command as `pp:method`): 555 GET, 202 PUT,
199 POST, 79 DELETE. Commands that only write local state (`sync`, `teach*`,
`learnings`, `playbook amend`, `profile`, `feedback`) are not listed.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, cross-org rollups, lookups. No Dashboard change. | `fleet health`, `fleet down`, `fleet renewals`, `fleet changes`, `fleet find`, `fleet fanout` (except at a Credential-tier path, below), `fleet admins`, `fleet flaps`, `fleet blame`, `fleet push` without `--apply` (its preview prints current credential fields as `[redacted]`), `sync`, `search`, and every GET command **except the secret-bearing ones in the Credential tier**. `search`, `analytics` and the MCP `sql` tool read the local mirror, which holds license keys and redacts credentials by field name and change-log label (see below) - allow them only if license keys, and anything that redaction could miss, are fine for the agent to see | Allow |
| **Write (routine)** | Day-to-day config changes on one network, device or org. | `networks update`, `networks wireless update-network-ssid`, `networks appliance update-network-vlan`, `devices update`, `networks snmp update-network`, `networks webhooks create-network-http-server`, `import`, and the other PUT/POST commands not listed below **that pass this test**: a write is routine only when it changes settings on one network, device or org without removing anything, replacing a membership list, changing who or what can sign in or call the API, or affecting power, firmware or connectivity. A write that fails the test is Bulk/disrupting, Destructive or Credential/security whether or not it is named below. **Rule:** a POST whose path ends in an action (`/reboot`, `/cycle`, `/swap`, `/rollbacks`, `/lock`, `/move`, `/moveSeats`, `/remove`, `/release`, `/claim`, `/split`, `/combine`, `/bind`, `/unbind`, `/wipe`, `/bulkDelete`, `/batchDelete`, `/claim/vmx`, `/renewSeats`, `/assignSeats`, `/moves`, `/migrations`, `/commands`, `/events`, `/add`, `/disenrollments`, `/detach`, `/disable`, `/uninstallApps`, `/installApps`, `/recalculate`, `/clone`, `/commit`, `/attach`, `/enable`, `/connect`, `/disconnect`, `/provision`, `/assign`, `/reassign`, `/bulkAssign`, `/bulkUnassign`, `/bulkCreate`, `/bulkUpdate`, `/batch`, `/batchCreate`, `/batchUpdate`, `/declare`, `/defer`, `/setDefault`, `/update`, `/wakeOnLan`, `/throughputTest`, `/dismiss`, `/restore`, `/stop`, `/cancel`, `/publish`) is never routine - treat it as Bulk/disrupting or Destructive even if it is not named below. A POST ending in `/generate` or `/generateSnapshot` returns a time-limited link to a packet capture or camera image: Credential tier | Preview with `--dry-run`, then a reviewed write |
| **Bulk and device-disrupting** | Takes effect on live client networks at once. | `fleet push --apply` (one change to many networks across orgs via action batches), `organizations action-batches create-organization-action-batch` and `update-organization-action-batch` (a raw action batch can change many networks in one call), `networks devices vmx-network-claim` (claims a vMX and consumes a license), `devices reboot device`, `devices switch cycle-device-ports`, `devices live-tools create-device-ports-cycle`, `devices live-tools create-device-cable-test`, `networks sm reboot-network-devices`, `networks sm shutdown-network-devices`, `networks appliance swap-network-warm-spare`, `networks appliance create-network-devices-redundancy-swap`, `networks firmware-upgrades update-network`, `networks firmware-upgrades create-network-rollback`, `networks switch create-network-stack` and `add-network-stack` (forming or growing a stack reboots the switches), `devices sensor create-device-command` (cycles or cuts power to whatever is plugged into an MT40 power controller), `networks firmware-upgrades create-network-staged-event`, `update-network-staged-events`, `defer-network-staged-events` and `rollbacks-network-staged-events` (schedule or change staged switch firmware upgrades, which reboot the switches), `networks sm lock-network-devices`, `networks sm uninstall-network-device-apps`, `organizations inventory create-organization-devices-swaps-bulk`, `organizations wireless recalculate-organization-radio-auto-rf-channels` (re-picks AP channels; wireless clients can drop briefly), `organizations switch clone-organization-devices` (overwrites the target switches' settings), `organizations policies commit-organization-global-group-firewall-rulesets-assignments` (applies firewall rulesets to their assigned networks) | Human-in-the-loop only. A port cycle drops whatever is plugged in, including PoE phones and APs |
| **Destructive** | Irreversible config, device or license loss. | Every DELETE command (79, including `organizations delete` and `networks delete`), `networks devices remove-network`, `organizations inventory release-from-organization`, `networks unbind network`, `networks split network`, `organizations networks combine-organization`, `networks sm wipe-network-devices`, `networks sm unenroll-network-device`, `networks switch remove-network-stack`, `networks switch update-network-stack` (with `members`, it replaces the stack's whole membership, so every switch left off the list is removed), `organizations wireless create-organization-zigbee-disenrollment` (removes door locks from the Zigbee network), `networks bind network` (overwrites the network's settings with a template), `organizations networks create-move` (moves networks to another org), `organizations devices create-organization-migration`, `organizations sase detach-organization-sites`, `organizations integrations disable-organization-xdr-networks`, `networks pii create-network-request` (files a PII deletion request), `organizations licenses move-organization`, `organizations licenses move-organization-seats`, `organizations licensing move-organization-coterm-licenses`, and every `.../bulkDelete` and `.../batchDelete` POST (for example `organizations sase batch-organization-connectors-delete`) | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Creates, revokes or RETURNS secrets and admin access. | `auth set-token`, `auth logout`, `administered generate-identities-me-api-keys`, `administered revoke-identities-me-api-keys`, `devices appliance create-device-vmx-authentication-token` (returns a vMX auth token), `networks sm create-network-bypass-activation-lock-attempt`, `organizations devices generate-organization-packet-capture-capture-download-url` and `devices camera generate-device-snapshot` (each returns a time-limited download or image link), and the reads that return one: `devices camera clip-device` (a video clip link), `networks floor-plans get-network` / `get-network-networks` (floor plan image links) and `organizations branding-policies get-organization` / `get-organization-branding-policy` (preview image links), every `organizations admins`, `organizations saml` and `organizations saml-roles` write, `organizations login-security update-organization` (login and API-key IP ranges and org-wide two-factor: it can lock every admin and API key out of the org, this CLI's own key included) and `organizations update` (`api.enabled=false` cuts every API key off the org). **Secret-returning reads** (GETs, but the CLI passes response bodies through unredacted): `networks wireless get-network-ssid` / `get-network-ssids` (Wi-Fi `psk`), `networks wireless get-network-ssid-identity-psk` / `get-network-ssid-identity-psks` (`passphrase`), `networks camera get-network-wireless-profile` / `get-network-wireless-profiles`, `networks snmp get-network` and `organizations snmp get-organization` (community strings), `organizations appliance get-organization-vpn-third-party-vpnpeers` (IPsec `secret`), `networks appliance get-network-vpn-bgp`, `networks switch get-network-routing-ospf`, `devices cellular get-device-sims`, `organizations sm get-organization-vpp-accounts` / `get-organization-vpp-account` (Apple VPP service token), `organizations configuration-changes get-organization` (the raw change log: an API change's request body, PSKs included, is in `oldValue`/`newValue`; `fleet changes` is the redacted view), `organizations action-batches get-organization` / `get-organization-action-batch` (return each action's body, so a batch or `fleet push --apply` that set a PSK or community string returns it), `networks webhooks get-network-payload-template` / `get-network-payload-templates` (header templates can carry an `Authorization` value), `networks webhooks get-network-http-server` / `get-network-http-servers` (a Slack or Teams incoming-webhook URL is itself a credential), `organizations webhooks get-organization-logs` (every delivery's receiver URL), `organizations webhooks get-organization-callbacks-status`, `devices live-tools get-device-ping` / `get-device-ping-device` (a job's `callback.url`), `networks webhooks get-network-test`, `networks alerts get-network-settings`, `organizations alerts get-organization-profiles`, `networks sensor get-network-alerts-profile` / `get-network-alerts-profiles` and `organizations assurance get-organization-alerts-profiles` (receiver URLs, or HTTP server IDs that are the base64 of the URL), and `organizations api-requests get-organization` (request paths and query strings can carry HTTP server IDs and receiver URLs). **License-key reads** (claimable entitlements, not logins): `organizations licenses get-organization` / `get-organization-organizations`, `organizations licensing get-organization-coterm-licenses`, `networks sm get-network-device-softwares` / `get-network-user-softwares` | Human-in-the-loop only. Do not put these in an agent's Allow list, and do not hand their raw output to a model |

## How to lock it down

- **Scope the key, because it is the real gate.** This CLI applies no privilege
  separation of its own: whatever the generating admin can do, any command can do.
  For reporting and triage, generate the key from a Dashboard admin with
  **read-only** organization access. Use a full-access key only for a workflow that
  must write, and only for the orgs it needs.
- **Turn on the MCP read-only switch for agents that should never write.** Set
  `CISCO_MERAKI_MCP_READ_ONLY=1` in the MCP server's environment (in Claude Desktop it is
  the "Read-only mode" setting). The server then hides the write commands, refuses
  every non-GET API call, and refuses `fleet push --apply`. It does not stop the
  secret-bearing GETs below, so it is a write guard, not a secrets guard.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above, and every `fleet push --apply`.
- **The rule, not the list: the CLI does not redact command output.** Any command that
  returns a stored field verbatim can return a secret. `fleet changes` and `fleet blame` are the
  exception: they redact credentials inside each change's old and new values. The raw
  change log itself is a secret-bearing read, because an API change logs its whole
  request body there (a PSK set over the API appears in `newValue`):
  `organizations configuration-changes get-organization`, `fleet fanout` at
  `organizations/{organizationId}/configurationChanges`, and MCP `cisco-meraki_execute`
  on that endpoint return it unredacted. Two paths reach the secret-bearing reads
  above without naming them: `fleet fanout` pointed at an org-scoped secret path (for
  example `organizations/{organizationId}/appliance/vpn/thirdPartyVPNPeers`,
  `organizations/{organizationId}/snmp`, `organizations/{organizationId}/sm/vppAccounts`,
  `organizations/{organizationId}/apiRequests` or `organizations/{organizationId}/alerts/profiles`)
  returns that secret for **every** org; and the
  writes on those same paths (for example `networks wireless update-network-ssid`,
  `networks snmp update-network`) echo the stored value back in their response. A
  `fleet push` preview prints each network's current value for every key in `--body`
  with credential fields (`psk`, a RADIUS `secret`, `communityString`) as `[redacted]`,
  and under a webhook path every current string (an HTTP server's `url`) as `[redacted]`.
- **The local mirror keeps license keys and redacts credentials.** `sync` stores the
  organization list and the org-level resources under it in a local SQLite file under
  your user account. Of the credential reads above, only the configuration change log
  and the API request log are synced: change-log old and new values are redacted
  before they are stored, and both logs' webhook HTTP server IDs and `url=` values are
  scrubbed (below). Write
  commands never store their response (it echoes the PSK or header the write set);
  organization-level data a write changes is refreshed by the next `sync`, and network
  and device settings are not in the mirror at all. A few reads run with the default
  `--data-source auto` (organization, network and device lookups and the
  `administered` identity and licensing-subscription reads) write their response
  through. Before any row is stored the CLI replaces every credential field (`psk`,
  `passphrase`, `password`, `secret`, `sharedSecret`, SNMP community strings and v3
  passwords, API keys and tokens, and the value of a header pair named like a
  credential, such as `Authorization` or `x-functions-key`, and the base64 webhook
  `headersFile` and `bodyFile`) with `[redacted]`; key names match with `_`, `-` and
  case ignored, and any key ending in `token` or `key` counts except `licenseKey`. A
  change-log value that is cut-off JSON is withheld whole as `[redacted: cut-off
  JSON]`, as is every webhook change's old and new value (payload templates are
  free-form, and an HTTP server's URL is a credential), and so is a change value that
  mentions a webhook (an action batch that set up a receiver), and every string under a
  `callback` object (an action batch's receiver URL and HTTP server ID). Webhook HTTP server IDs
  (the base64 of the receiver URL, anywhere a base64 string decodes to a URL) in change-log labels, values and API request paths, and
  `url=` query values, are replaced with `[redacted:<short hash>]` (so two receivers
  stay distinct without revealing either) in the mirror, in row IDs and in
  `fleet changes` and `fleet blame` output; these match percent-encoded too. A `url=`
  value in an API request's query string or path, or in a change-log label, is withheld to the end of that string,
  pagination parameters after it included, because a receiver URL's own parameters
  cannot be told apart from the request's. `sync` skips the webhook delivery logs,
  whose rows carry receiver URLs. The redaction works by field name, change-log label
  and these patterns, and other receiver URLs (Location Scanning) are kept, so treat
  the mirror as sensitive anyway. License keys are kept on
  purpose - the org's licenses and its co-term licenses - so `search`, `analytics` and
  the MCP `sql` tool can return them from the mirror. Keep the mirror under your
  account and out of shared folders.
- **The CLI's response cache keeps credential reads on disk.** A repeat GET within 5
  minutes is answered from an on-disk cache (files readable only by your account) and
  still reports `"source": "live"`, so right after a key rotation it can return the
  old secret. The cached file stays on disk after the 5 minutes, until the same GET
  overwrites it or a write clears the cache. Pass `--no-cache` on credential reads so
  nothing is cached. The MCP server does not use this cache.
- **Never let an agent run Bulk, Destructive, or Credential tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the key if it is ever exposed** (for example after bridging the MCP server
  to a public endpoint for ChatGPT - see mcp-install.md). Keys are revoked and
  regenerated under Dashboard > My profile > API access.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the key, the binary uses it against the Meraki
Dashboard API, and you can read every line of how it does so. The skill is read-first,
preview-by-default for bulk changes, and scoped to whatever your Dashboard admin
account can already reach.
