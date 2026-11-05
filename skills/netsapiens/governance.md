# netsapiens skill - governance and safety model

> Unofficial. Community-built skill for the NetSapiens API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the netsapiens skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `netsapiens-cli` binary (and `netsapiens-mcp`),
authenticating with `NETSAPIENS_API_KEY` (an API key or OAuth2 access token) against the platform host in
`NETSAPIENS_SERVER`. The key is read from the environment, or from the credential store `auth set-token`
writes (a file only your user can read). It is never logged, never printed, never stored in the local mirror,
and never sent anywhere except your NetSapiens platform.

## Default-safe behavior

- **MCP writes need `confirm: true`.** Every MCP write tool previews the request first and sends only when called again with `confirm: true`. `NETSAPIENS_MCP_READ_ONLY=1` refuses every write.
- **`--dry-run` is opt-in on the CLI - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
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
| **Read** | Audits, lookups, search. No change. | `fleet seats`, `fleet offline`, `fleet dids`, `fleet e911`, `fleet missed`, `fleet queues`, `fleet changes`, `trace`, `search`, and the list, get and count commands | Allow |
| **Local-only writes** | Write to your machine only. | `sync`, `fleet changes --rebaseline` (CLI only), `auth set-token`, `auth logout` | Allow (no tenant change) |
| **Write (routine)** | Day-to-day changes to users, numbers, queues and phones. | `domains users create`, `domains users update`, `domains phonenumbers update`, `domains callqueues agents-create`, `phones create` | Preview with `--dry-run`, then an approved write |
| **Live calls and voicemail** | Act on a client's live calls and mailboxes. | `domains users calls-create`, `calls-transfer`, `calls-delete`, `voicemails-delete` | Human-in-the-loop only |
| **Credential / security** | Create, read or revoke keys, tokens, certificates and event subscriptions. | `apikeys`, `jwt`, `tokens`, `auth-code`, `sso-enroll`, `certificates`, `firebase`, `subscriptions` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | every `delete` command, `domains delete`, `restore update`, `backup`, `domains backup create` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers; NetSapiens keys can be read-only.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Live-call or Destructive tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the NetSapiens API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
