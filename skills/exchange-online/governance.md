# exchange-online skill - governance and safety model

> Unofficial. Community-built skill for the Microsoft Exchange Online API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the exchange-online skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `exchange-online-cli` binary (and `exchange-online-mcp`),
authenticating with `EXCHANGE_ONLINE_TOKEN`: a bearer token for `https://outlook.office365.com`
issued for one customer tenant. The token comes from the environment or from the CLI's own
credentials file (written by `auth set-token`, owner-only permissions); it is sent only to the
configured Exchange Online host. Requests to any other host, redirects and off-host paging links
are refused, the on-disk response cache is off, and a held credential is withheld from
every output, file and outbound request (feedback endpoint, `--deliver` webhook).

## Default-safe behavior

- **MCP writes preview until confirmed.** Every state-changing cmdlet tool and `offboard`
  return a preview (offboard returns its `--plan` report) until the call carries
  `confirm: true`. `EXCHANGE_ONLINE_MCP_READ_ONLY=1` removes every write tool and keeps
  `exchange-online_read` for the Get-* cmdlets.
- **On the command line, `--dry-run` is opt-in - use it.** Mutating commands send immediately
  unless you pass `--dry-run` (or `--plan` for `offboard`) first. Make your agent's policy:
  preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (lists, audits, traces); they cannot change
  anything, even though every Exchange admin call is an HTTP POST.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add any write
  gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lists, audits, traces. No change. | `mailboxes list`, `inbox-rules list`, `inbox-rules sweep` (removed over MCP in read-only mode), `forwarding-audit`, `ews-posture`, `bec triage`, `access-map`, `message-trace list`, `where-is`, `quarantine list`, `quarantine queue`, every policy and connector list | Allow |
| **Write (routine)** | Day-to-day mailbox and mail-flow changes. | `mailboxes set`, `mailbox-permissions add`, `send-as add`, `send-on-behalf set`, `folder-permissions add`, `folder-permissions set`, `auto-reply set`, `client-access set`, `distribution-groups add-member`, `transport-rules enable`, `quarantine release`, `blocked-senders remove` | Preview with `--dry-run`, then an approved write (over MCP, `confirm: true`) |
| **Credential / security** | Saves or clears the stored token. | `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Removes access, data or devices. | `mobile-devices wipe`, `mobile-devices remove`, `quarantine delete`, `inbox-rules remove`, `mailbox-permissions remove`, `send-as remove`, `folder-permissions remove`, `distribution-groups remove-member`, `transport-rules disable`, `offboard`, `local-data purge --yes` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none) | Operator-only, not for agents |

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
the Microsoft Exchange Online API, and you can read every line of how it does so. The skill is
read-first, previews every MCP write until confirmed, and is scoped to the tenant and role your token carries.
