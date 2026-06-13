# connectwise-control skill - governance and safety model

> Unofficial. Community-built skill for the ConnectWise Control (ScreenConnect) API.
> Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.
> This page tells an MSP owner exactly what the connectwise-control skill can touch and
> how to scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `connectwise-control-cli` binary (and `connectwise-control-mcp`),
authenticating with HTTP Basic auth against your instance: `CONNECTWISE_CONTROL_BASE_URL`
(your ScreenConnect host), `CONNECTWISE_CONTROL_USERNAME`, and
`CONNECTWISE_CONTROL_PASSWORD` - the same login you use for the web console. Credentials
are read from the environment only - never written to disk by the skill, never logged,
never sent anywhere except your own ConnectWise Control instance.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (listing and inspecting sessions, users, the audit log, search); they cannot change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The most important thing about this connector: some commands **act on a real remote
machine**. Running a command on a guest endpoint is remote code execution on a
customer's computer. The safe default for an autonomous agent is **read only**; require
a human for any session/host control, access grant, or user-management command.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lists and inspects sessions, groups, users, and the audit log. No change. | `sessions list`, `sessions get-detail`, `session-groups`, `security get-configuration`, `audit get-info`, `audit query-log`, `search` | Allow |
| **Write (session metadata)** | Changes a session's name or custom properties. No effect on the remote machine. | `sessions update-name`, `sessions update-custom-property` | Preview with `--dry-run`, then an approved write |
| **Session and host control** | Acts on a real guest machine - remote code execution and control events. | `sessions run-command` (runs a real command on the guest endpoint), `sessions add-event-to` (queues control events such as Wake-on-LAN) | Human-in-the-loop, explicit confirmation |
| **Access grant** | Issues a token that grants remote access to a session. | `sessions get-access-token` (one-time join token / URL) | Human-in-the-loop only |
| **Admin and identity** | Creates, updates, or deletes instance users and roles. | `security save-user`, `security delete-user` | Operator-only, not for agents |

> Note: `security get-configuration` returns the instance's users and roles. It is
> read-only and tiered Read, but it exposes account structure - scope the credential so
> an agent that only needs to list sessions cannot also enumerate or edit users.

## How to lock it down

- **Scope the instance user** to only what your workflow needs. A monitoring/triage
  workflow does not need a Control login that can run commands on guests, issue access
  tokens, or manage users.
- **Keep autonomous agents to the Read tier.** Have a human approve any Session/host
  control, Access grant, Write, or Admin command - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Treat `sessions run-command` like a production change on a customer's machine.** It
  executes a real command on a live endpoint: human, reviewed, logged. Never let an agent
  run it unattended.
- **Rotate the password if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the login, the binary uses it against your own
ConnectWise Control instance, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
