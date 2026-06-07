# domotz skill - governance and safety model

> Unofficial. Community-built skill for the Domotz API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the domotz skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `domotz-cli` binary (and `domotz-mcp`),
authenticating with `DOMOTZ_API_KEY` (or `DOMOTZ_PUBLIC_API_KEY`) and the
`DOMOTZ_REGION` that selects your portal's API cell. Credentials are read from the
environment only - never written to disk, never logged, never sent anywhere except
the Domotz API.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Most read commands are safe to run** (reports, rollups, search); they cannot
  change anything. The one exception is `agent device get-snmpauthentication`,
  which returns SNMP community strings and keys - treat it like a credential, not a
  routine read (see the Credential tier below).
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line. Domotz uses several action verbs
(`edit`, `hide`, `apply`, `move`, `bind`, `add`, `import`, plus the power-control
verbs) that are mutations even though they don't read like one - they are listed
under Write below so an agent never mistakes them for safe reads.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, status, history, counts. No change. | `fleet health`, `fleet offline`, `fleet inventory`, `agent list`, `agent device list`, `agent device get`, `search`, `topology`, `export` | Allow |
| **Write (routine)** | Day-to-day mutations and device control. | `agent device create-eye-snmp`, `agent device create-eye-tcp`, `agent device edit`, `agent device hide`, `agent device set-inventory-field-value`, `agent device update-monitoring-state`, `custom-tag create`, `custom-tag edit`, `alert-profile binding bind-alert-profile-to-device`, `device-profile apply device-profile`, `agent network add-excluded-device`, `agent ownership move-agent`, `import` | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Physical / device control** | Acts on real hardware or live connections. | `agent device power-action-on`, `agent device trigger-outlet-action`, `agent device attach-to-outlet`, `agent device detach-from-outlet`, `agent device backup-configuration`, `agent device onvif-snapshot`, `agent device connect-to`, `agent connection create-agent-vpnconnection` | Human-in-the-loop; never unattended |
| **Credential / security** | Reads or writes secrets, or manages who can log in. | `agent device set-credentials`, `agent device set-snmpauthentication`, `agent device set-snmp-community`, `agent device get-snmpauthentication` (returns SNMP secrets), `rbac create-user`, `rbac edit-user`, `rbac create-user-group`, `rbac edit-user-group` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `agent delete`, `agent device delete`, `agent device delete-down`, `agent device delete-eye-snmp`, `agent device delete-eye-tcp`, `custom-driver association delete-custom-driver`, `inventory delete`, `rbac delete-user`, `rbac delete-user-group`, `alert-profile binding unbind-alert-profile-from-device` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive, Physical, or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Physical, Credential, or Destructive tier commands
  unattended.** Powering an outlet off or deleting a Collector is real-world impact;
  treat them like a production database drop: human, reviewed, logged.
- **Treat `get-snmpauthentication` like a password read.** It returns SNMP community
  strings and keys; route it through the same approval as the credential setters.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Domotz API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
