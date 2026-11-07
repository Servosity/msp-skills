# threecx skill - governance and safety model

> Unofficial. Community-built skill for the 3CX API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the threecx skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `threecx-cli` binary (and `threecx-mcp`),
authenticating to each 3CX phone system with an OAuth client-credentials API client
(Admin Console > Integrations > API): `THREECX_CLIENT_ID` (the DN number shown there),
`THREECX_CLIENT_SECRET` (the API key) and `THREECX_FQDN` for a single PBX, or one
registered PBX per client (`fleet add`) whose API key is read from an environment
variable you name. The fleet registry stores that variable's name, never the key. A
minted access token is kept only in the CLI's private (0600) credential file. No
credential is printed, logged or written to any other file, and no request or
redirect may leave the configured phone system.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Fleet reports and every GET command. No change on the phone system. | `fleet health`, `fleet trunks`, `fleet licences`, `fleet versions`, `fleet backups`, `fleet changes`, `fleet exec` (GET only), `users list`, `trunks list`, `system-status`, `search` | Allow |
| **Write (routine)** | Day-to-day configuration changes (PATCH and POST actions). | `users-id update-user`, `queues-id update-queue`, `holidays-id update-holiday`, `office-hours update`, `contacts-id update-contact`, `import` | Preview with `--dry-run`, then an approved write. In the MCP server every write previews until `confirm: true` |
| **Phone-system control** | Acts on the running PBX, its phones or live calls. | `services restart`, `services stop`, `services restart-operating-system`, `install-updates`, `set-update-settings`, `users reboot-phone`, `users reprovision-all-phones`, `sip-devices bulk-update-firmware`, `firmwares-id push-firmware-for-phones`, `backups-file-name restore`, `users make-call`, `active-calls-id` (drops a call), `system-status revoke-remote-access` | Human-in-the-loop only |
| **Credential / identity** | Any command that can return, mint, rotate or revoke a credential, and API client and user identity management. | `conference-settings generate-api-key`, `system-status apitoken`, `service-principals-id generate-app-token`, `users-id generate-prov-link`, `my-user generate-prov-link`, `microsoft365-integration get-microsoft-access-token`, `crm-integration get-oauth`, `users regenerate-passwords`, `fxs-mac-address regenerate-web-credentials`, `license-status replace-license-key`, `service-principals create`, `users create`, `oauth-clients`, `my-tokens`, `security-tokens`, `auth login` | Human-in-the-loop only. The CLI prints a notice in place of any minted value |
| **Destructive** | Irreversible data or configuration loss. | every DELETE and purge: `users-id delete-user`, `users batch-delete`, `purge-calls`, `purge-chats`, `purge-all-logs`, `recordings purge-local`, `recordings purge-archive`, `backups purge-local`, `firmwares clean-up`, `outbound-rules purge`, `fleet remove`, `local-data purge` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the 3CX API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
