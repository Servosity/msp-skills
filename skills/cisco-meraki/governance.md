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
| **Read** | Reports, cross-org rollups, lookups. No Dashboard change. | `fleet health`, `fleet down`, `fleet renewals`, `fleet changes`, `fleet find`, `fleet fanout` (except at a Credential-tier path, below), `fleet admins`, `fleet flaps`, `fleet blame`, `fleet push` without `--apply`, `sync`, `search`, and every GET command **except the secret-bearing ones in the Credential tier**. `search`, `analytics` and the MCP `sql` tool read the local mirror, which holds license keys but never credentials (see below) - allow them only if license keys are fine for the agent to see | Allow |
| **Write (routine)** | Day-to-day config changes on one network, device or org. | `networks update`, `networks wireless update-network-ssid`, `networks appliance update-network-vlan`, `devices update`, `networks snmp update-network`, `networks webhooks create-network-http-server`, `import`, and the other PUT/POST commands not listed below. **Rule:** a POST whose path ends in an action (`/reboot`, `/cycle`, `/swap`, `/rollbacks`, `/lock`, `/move`, `/moveSeats`, `/remove`, `/release`, `/claim`, `/split`, `/combine`, `/bind`, `/unbind`, `/wipe`, `/bulkDelete`, `/batchDelete`, `/claim/vmx`, `/renewSeats`, `/assignSeats`, `/moves`, `/migrations`, `/detach`, `/disable`, `/uninstallApps`) is never routine - treat it as Bulk/disrupting or Destructive even if it is not named below | Preview with `--dry-run`, then a reviewed write |
| **Bulk and device-disrupting** | Takes effect on live client networks at once. | `fleet push --apply` (one change to many networks across orgs via action batches), `organizations action-batches create-organization-action-batch` and `update-organization-action-batch` (a raw action batch can change many networks in one call), `networks devices vmx-network-claim` (claims a vMX and consumes a license), `devices reboot device`, `devices switch cycle-device-ports`, `devices live-tools create-device-ports-cycle`, `devices live-tools create-device-cable-test`, `networks sm reboot-network-devices`, `networks sm shutdown-network-devices`, `networks appliance swap-network-warm-spare`, `networks appliance create-network-devices-redundancy-swap`, `networks firmware-upgrades update-network`, `networks firmware-upgrades create-network-rollback`, `networks sm lock-network-devices`, `networks sm uninstall-network-device-apps`, `organizations inventory create-organization-devices-swaps-bulk` | Human-in-the-loop only. A port cycle drops whatever is plugged in, including PoE phones and APs |
| **Destructive** | Irreversible config, device or license loss. | Every DELETE command (79, including `organizations delete` and `networks delete`), `networks devices remove-network`, `organizations inventory release-from-organization`, `networks unbind network`, `networks split network`, `organizations networks combine-organization`, `networks sm wipe-network-devices`, `networks sm unenroll-network-device`, `networks switch remove-network-stack`, `networks bind network` (overwrites the network's settings with a template), `organizations networks create-move` (moves networks to another org), `organizations devices create-organization-migration`, `organizations sase detach-organization-sites`, `organizations integrations disable-organization-xdr-networks`, `networks pii create-network-request` (files a PII deletion request), `organizations licenses move-organization`, `organizations licenses move-organization-seats`, `organizations licensing move-organization-coterm-licenses`, and every `.../bulkDelete` and `.../batchDelete` POST (for example `organizations sase batch-organization-connectors-delete`) | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Creates, revokes or RETURNS secrets and admin access. | `auth set-token`, `auth logout`, `administered generate-identities-me-api-keys`, `administered revoke-identities-me-api-keys`, `devices appliance create-device-vmx-authentication-token` (returns a vMX auth token), `networks sm create-network-bypass-activation-lock-attempt`, every `organizations admins`, `organizations saml` and `organizations saml-roles` write. **Secret-returning reads** (GETs, but the CLI passes response bodies through unredacted): `networks wireless get-network-ssid` / `get-network-ssids` (Wi-Fi `psk`), `networks wireless get-network-ssid-identity-psk` / `get-network-ssid-identity-psks` (`passphrase`), `networks camera get-network-wireless-profile` / `get-network-wireless-profiles`, `networks snmp get-network` and `organizations snmp get-organization` (community strings), `organizations appliance get-organization-vpn-third-party-vpnpeers` (IPsec `secret`), `networks appliance get-network-vpn-bgp`, `networks switch get-network-routing-ospf`, `devices cellular get-device-sims`, `organizations sm get-organization-vpp-accounts` / `get-organization-vpp-account` (Apple VPP service token). **License-key reads** (claimable entitlements, not logins): `organizations licenses get-organization` / `get-organization-organizations`, `organizations licensing get-organization-coterm-licenses`, `networks sm get-network-device-softwares` / `get-network-user-softwares` | Human-in-the-loop only. Do not put these in an agent's Allow list, and do not hand their raw output to a model |

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
- **The rule, not the list: the CLI never redacts output.** Any command that returns a
  stored field verbatim can return a secret. Three paths reach the secret-bearing reads
  above without naming them: `fleet fanout` pointed at an org-scoped secret path (for
  example `organizations/{organizationId}/appliance/vpn/thirdPartyVPNPeers`,
  `organizations/{organizationId}/snmp` or `organizations/{organizationId}/sm/vppAccounts`)
  returns that secret for **every** org; a
  `fleet push` preview prints each network's **current** value for every key in
  `--body`, so a push touching `psk` or `communityString` prints the live one; and the
  writes on those same paths (for example `networks wireless update-network-ssid`,
  `networks snmp update-network`) echo the stored value back in their response.
- **The local mirror keeps license keys, never credentials.** `sync` stores the
  organization list and the org-level resources under it in a local SQLite file under
  your user account; none of the credential reads above is synced. Reads and writes
  run with the default `--data-source auto` also write their response through to the
  mirror, so before any row is stored the CLI replaces every credential field
  (`psk`, `passphrase`, `password`, `secret`, `sharedSecret`, SNMP community strings
  and v3 passwords, API keys and tokens) with `[redacted]`. License keys are kept on
  purpose - the org's licenses and its co-term licenses - so `search`, `analytics` and
  the MCP `sql` tool can return them from the mirror. Keep the mirror under your
  account and out of shared folders.
- **The CLI's response cache can hold a credential read for 5 minutes.** A repeat GET
  within 5 minutes is answered from an on-disk cache (files readable only by your
  account) and still reports `"source": "live"`, so right after a key rotation it can
  return the old secret. Pass `--no-cache` on credential reads. The MCP server does
  not use this cache.
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
