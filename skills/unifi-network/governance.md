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
sees one controller. Credentials are read from the environment (or a config file you
write with `auth set-token`) - never logged, and never sent anywhere except your own
gateway. Nothing routes through a vendor cloud.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass
  `--dry-run` first to preview the request without sending. Make your agent's policy:
  preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (the local-mirror views, reports, search);
  they cannot change anything.
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
| **Read** | Reports, rollups, local-mirror views, search. No change. | `drift`, `newcomer`, `topology`, `port-audit`, `guest report`, `rule-predict`, `search`, `analytics`, and every non-mutating `sites ...` endpoint | Allow |
| **Write (routine)** | Day-to-day config mutations. 18 commands. | `sites acl-rules create`, `sites acl-rules update`, `sites acl-rules update-ordering`, `sites dns create-policy`, `sites dns update-policy`, `sites firewall create-policy`, `sites firewall create-zone`, `sites firewall patch-policy`, `sites firewall update-policy`, `sites firewall update-policy-ordering`, `sites firewall update-zone`, `sites hotspot create-vouchers`, `sites networks create`, `sites networks update`, `sites traffic-matching-lists create`, `sites traffic-matching-lists update`, `sites wifi create-broadcast`, `sites wifi update-broadcast` | Preview with `--dry-run`, then an approved write |
| **Device / port control** | Takes physical effect on the network right now. 4 commands. | `sites devices adopt`, `sites devices execute-adopted-action`, `sites devices execute-port-action`, `sites clients execute-connected-action` | Human-in-the-loop only. A port action can power-cycle PoE and drop whatever is plugged into it; a client action can force a device off the network |
| **Destructive** | Irreversible config or hardware loss. 10 commands. | `sites devices remove` (**unadopts the device, and factory-resets it if it is online**), `sites acl-rules delete`, `sites dns delete-policy`, `sites firewall delete-policy`, `sites firewall delete-zone`, `sites hotspot delete-voucher`, `sites hotspot delete-vouchers`, `sites networks delete`, `sites traffic-matching-lists delete`, `sites wifi delete-broadcast` | Human-in-the-loop only, explicit confirmation |
| **Credential / security** | Touches tokens, keys, MFA. | `auth set-token`, `auth logout` (local credential storage only - no remote key management in this API) | Human-in-the-loop only |

## How to lock it down

- **Treat the API key as gateway-admin.** The UniFi integration API key is not
  scopeable to read-only, so the same credential that runs `drift` can run
  `sites devices remove`. The gate has to live in your agent's policy, not in the
  key. If you want a read-only agent, restrict which commands it may call.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above.
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
