# HaloPSA skill - governance and safety model

> Unofficial. Community-built skill for the HaloPSA, HaloITSM, and HaloCRM APIs.
> Not affiliated with, endorsed by, or sponsored by Halo Service Solutions Ltd.
> This page tells an MSP owner what the skill can touch and how to scope it.

## What it authenticates as

The skill drives `halopsa-cli` (and the `halopsa-mcp` server), authenticating to
**your own HaloPSA tenant** with an OAuth application you create at
Configuration > Integrations > Halo PSA API. You provide `HALOPSA_CLIENT_ID`,
`HALOPSA_CLIENT_SECRET`, and `HALOPSA_TENANT` (optionally a pre-issued
`HALOPSA_TOKEN`). Credentials are read from the environment only - never written
to disk, never logged, never sent anywhere except your Halo endpoint.

## The permission lever lives in Halo, not in this skill

The single most important control is **the scope you grant the OAuth application
in your Halo tenant.** The CLI can only do what that application is permitted to
do. When you create the API application:

- Grant the **minimum permissions** your workflow needs. A read/report workflow
  (queue triage, SLA checks, client cards, contract burn-down) does not need
  write or delete permissions.
- Use a **dedicated application** for the agent, not a shared admin integration,
  so you can audit and revoke it independently.
- Revoke or rotate the client secret immediately if it is ever exposed (for
  example after bridging the MCP server to a public HTTPS endpoint for ChatGPT -
  see [mcp-install.md](./mcp-install.md)).

## Default-safe behavior

The CLI is built to avoid silent changes (full contract in [AGENTS.md](./AGENTS.md)):

- **Discovery first.** `halopsa-cli doctor` and `agent-context` report runtime
  truth before any action.
- **Plan before mutate.** For an unfamiliar command that may change remote state,
  the agent is instructed to inspect `--help` and prefer `--dry-run` first.
- **Explicit non-interactive writes.** `--yes --no-input` is reserved for after
  the target, arguments, and side effects are understood.
- **Read commands are safe.** The cross-entity views (`triage`, `tickets`,
  `sla breaching`, `client card`, `contracts burn`) read and aggregate; they do
  not change Halo state.

## Recommended agent policy

| Tier | What it does | Recommended agent policy |
| --- | --- | --- |
| **Read** | Queue/SLA/asset/contract reporting and cross-entity views | Allow |
| **Write (routine)** | Ticket updates, notes, actions, assignment | Allow with `--dry-run` preview, then a reviewed write |
| **Destructive / config** | Deletes and configuration changes | Human-in-the-loop only; not for an autonomous agent |

Keep autonomous agents to **Read plus previewed writes**, and gate anything
destructive behind a human. Because Halo enforces the OAuth application's scope
server-side, the strongest guarantee is simply not granting the application
permissions you do not want an agent to have.

## Why an MSP owner can be comfortable

You own the tenant, you create and scope the OAuth application, and the full
source of the CLI and MCP server is in this repository under [`cli/`](./cli)
(Apache-2.0). The credential path is auditable end to end, and the skill defaults
to discovery and dry-run rather than blind mutation.
