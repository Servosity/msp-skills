# unifi-network skill - governance and safety model

> Unofficial. Community-built skill for the UniFi Network integration API. Not
> affiliated with, endorsed by, or sponsored by Ubiquiti Inc.
> This page tells an MSP owner exactly what the unifi-network skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `unifi-network-cli` binary (and `unifi-network-mcp`) against a
**single self-hosted UniFi OS gateway on your own network**. Two environment
variables are required:

- `UNIFI_API_KEY` - a local API key minted in the gateway's own UI
  (Settings > Control Plane > Integrations > Create API Key).
- `UNIFI_GATEWAY_HOST` - the gateway's hostname or IP, e.g. `10.0.0.1`. The CLI
  builds `https://<host>/proxy/network` from it. The integration spec declares no
  absolute server, so there is no default endpoint to fall back on.

The key is gateway-scoped, not a cloud or multi-tenant credential: one CLI instance
sees one controller. Credentials are read from the environment (or from `credentials.toml` under the data dir,
which `auth set-token` writes; a legacy `config.toml` is still read for compatibility) - never logged, and never sent anywhere except your own
gateway. Nothing routes through a vendor cloud.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass
  `--dry-run` first to preview the request without sending. Make your agent's policy:
  preview, show the exact command, get approval, then run the write.
- **Most read commands are safe to run** (the local-mirror views, reports, search); they
  change nothing on the gateway. Two caveats. `drift` and `newcomer` write their own
  local baseline on every run - `drift` advances its snapshot, so running it twice in a
  row makes the second run report no changes. And several reads return **secrets** - the
  WiFi detail read, the hotspot voucher reads, `guest report`, and `search`; see the
  Credential tier below.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.
- **Local-mirror commands read your own disk.** `drift`, `newcomer`, `topology`,
  `guest report`, `rule-predict`, and `search` compute from the synced SQLite mirror.
  `port-audit` is the exception: it reads the device list locally but fetches port
  detail live, one call per switching or gateway device.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below that line. This table covers all 32 commands that mutate the
gateway, plus local credential storage. It does not cover commands that only write
local state (`sync`, `teach*`, `learnings`, `playbook amend`, `profile`, `feedback`).

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, local-mirror views. No gateway change. | `drift`, `newcomer`, `topology`, `port-audit`, `rule-predict`, and non-mutating `sites ...` endpoints **except the secret-bearing commands in the Credential tier** | Allow |
| **Write (routine)** | Day-to-day config mutations. 18 commands. | `sites acl-rules create`, `sites acl-rules update`, `sites acl-rules update-ordering`, `sites dns create-policy`, `sites dns update-policy`, `sites firewall create-policy`, `sites firewall create-zone`, `sites firewall patch-policy`, `sites firewall update-policy`, `sites firewall update-policy-ordering`, `sites firewall update-zone`, `sites hotspot create-vouchers`, `sites networks create`, `sites networks update`, `sites traffic-matching-lists create`, `sites traffic-matching-lists update`, `sites wifi create-broadcast`, `sites wifi update-broadcast` | Preview with `--dry-run`, then an approved write |
| **Device / port control** | Takes physical effect on the network right now. 4 commands. | `sites devices adopt`, `sites devices execute-adopted-action`, `sites devices execute-port-action`, `sites clients execute-connected-action` | Human-in-the-loop only. A port action can power-cycle PoE and drop whatever is plugged into it; a client action can force a device off the network |
| **Destructive** | Irreversible config or hardware loss. 10 commands. | `sites devices remove` (**unadopts the device, and factory-resets it if it is online**), `sites acl-rules delete`, `sites dns delete-policy`, `sites firewall delete-policy`, `sites firewall delete-zone`, `sites hotspot delete-voucher`, `sites hotspot delete-vouchers`, `sites networks delete`, `sites traffic-matching-lists delete`, `sites wifi delete-broadcast` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Handles or RETURNS secrets. | Local credential storage: `auth set-token`, `auth logout`. **Secret-returning reads** (they are `GET`s or local-mirror reads, but the output carries a live secret and the CLI does not redact response bodies): `sites wifi get-broadcast-details` returns that SSID's `securityConfiguration`, which for WPA2/WPA3-Personal contains the cleartext `passphrase` (the list endpoint `get-broadcast-page` returns a security-type discriminator and preshared-key network ids, but no passphrase). `sites hotspot get-voucher`, `sites hotspot get-vouchers`, **`guest report`**, **`search`**, **`analytics`** (`--type hotspot --group-by code` prints every code as a group label), and **`export <resource>`** (writes the whole resource to a file) all surface usable guest voucher `code` values, most of them from the local mirror rather than the API | Human-in-the-loop only. Do not put these in an agent's Allow list, and do not pipe their raw output into a model's context |

## How to lock it down

- **Treat the API key as gateway-admin.** This CLI applies no privilege separation of
  its own: whatever the key is permitted to do, any command can do, so the same
  credential that runs `drift` may also run `sites devices remove`. Check what the key
  you minted can actually reach in the gateway UI, and put the real gate in your agent's
  policy by restricting which commands it may call.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above.
- **Secrets come back from three WRITES too.** Three **writes** also return secrets in their response body and deserve the same handling: `sites hotspot create-vouchers` returns the new voucher `code`s, and `sites wifi create-broadcast` / `update-broadcast` return the SSID's `securityConfiguration` including the cleartext passphrase. They stay in the Write tier for
  blast radius, but their OUTPUT is credential-grade - do not log it or hand it to a model.
- **The rule, not the list: the CLI never redacts output**, so ANY command that can emit a
  stored or returned field verbatim can emit a secret. This API has two - an SSID's
  cleartext `passphrase` and a guest voucher `code`. The enumerations below are the
  carriers known today; treat a newly added mirror-reading command as one until proven
  otherwise.
- **Exclude the secret-returning reads from any blanket read allowance.** "Allow all
  GETs" is not a safe policy on this API, because the CLI passes response bodies through
  unredacted: `sites wifi get-broadcast-details <siteId> <wifiBroadcastId>` returns that
  SSID's pre-shared key in cleartext (enumerate the broadcasts and it is every key on the
  site), and `guest report` / `search` hand back live guest voucher codes from the mirror
  without touching the gateway at all.
- **`sync` writes secrets to disk.** It mirrors `/v1/sites/{siteId}/hotspot/vouchers`, so
  usable guest voucher codes land in `data.db` in cleartext and stay readable offline.
  Treat the mirror as credential-bearing: it lives under your user account, and deleting a
  voucher on the gateway does not scrub the synced copy until you re-sync.
- **TLS verification is OFF by default for private gateways.** For any RFC1918, loopback,
  or link-local host the CLI skips certificate verification (that is what makes the
  gateway's self-signed cert work with no flags). It is the right default on a LAN, but it
  means the connection is not authenticated - set `UNIFI_INSECURE_SKIP_VERIFY=0` to force
  verification back on if you have installed a real certificate.
- **Never let an agent run Device/port control or Destructive commands unattended.**
  `execute-port-action` and `sites devices remove` have immediate physical
  consequences on a live network - treat them like a production database drop:
  human, reviewed, logged.
- **Rotate the key if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). Keys are
  revoked and reminted in the same gateway UI screen that created them.
- **The gateway is on your LAN.** Exposing the MCP server publicly effectively
  exposes an admin path to your network. Bridge it deliberately.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
your own gateway, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to one controller you already administer.
