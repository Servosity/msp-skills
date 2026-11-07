# microsoft-graph skill - governance and safety model

> Unofficial. Community-built skill for the Microsoft Graph API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the microsoft-graph skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `microsoft-graph-cli` binary (and `microsoft-graph-mcp`),
authenticating with `MICROSOFT_GRAPH_TOKEN` from the environment, or with the token
`auth login` mints and keeps in the 0600 credentials file in the data directory.
Credentials are never logged and are sent only to the Microsoft Graph API; the client
secret goes only to the Entra token endpoint. Any output that would repeat a credential
is withheld.

## Default-safe behavior

- **No command writes to your tenant.** Directory, licensing, security, device, Teams
  Phone and the cross-entity analytics (`licenses waste`, `admins audit`, `apps consent`,
  `security triage`, `managed-devices drift`, `groups risk`, `tenant snapshot`) only read -
  from the live Graph API or the local SQLite mirror.
- **Transcripts are own tenant only.** `transcripts list`, `transcripts download` and
  `transcripts all` refuse any token whose tenant is not `MICROSOFT_GRAPH_HOME_TENANT`, a
  user who signed in from another tenant (GDAP or guest), and an app-only token unless
  `MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1` and a Teams application access policy limits the
  app to named users. Nothing is kept on disk; through MCP they return text and never write
  a file, and `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` switches them off.
- **Local writes stay local.** `auth login`, `profile save`, the learn commands and
  `local-data purge --yes` change only this machine's credential file, profiles, learn store
  or local data. `MICROSOFT_GRAPH_NO_LOCAL_DATA=1` keeps no local copy of API responses.
- **MCP over HTTP needs a token.** `microsoft-graph-mcp --transport http` refuses to start
  until `MICROSOFT_GRAPH_MCP_HTTP_TOKEN` is set, and any address other than loopback also
  needs TLS. See mcp-install.md.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, audits, search. No change. | `licenses waste`, `licenses orphans`, `admins audit`, `apps consent`, `security triage`, `managed-devices drift`, `groups risk`, `tenant snapshot`, `users list`, `phone-numbers list`, `teams-users list`, `pull`, `search`, `export` | Allow |
| **Sensitive read** | Meeting content, own tenant only, never stored. | `transcripts list`, `transcripts download`, `transcripts all` | Allow only for your own meetings; switch off for MCP with `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` |
| **Local writes** | This machine only. | `auth login`, `auth logout`, `profile save`, `teach`, `local-data purge --yes` | Allow |
| **Credential / security** | Touches tokens, keys, MFA in the tenant. | (none - `auth login` only mints and keeps your own token locally) | Human-in-the-loop only |
| **Tenant writes / destructive** | Creates, changes or deletes tenant data. | (none - the CLI exposes no create, update or delete path) | Not applicable |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep transcripts for your own meetings.** Leave `MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY`
  unset unless a Teams application access policy limits the app, and set
  `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` if your agents do not need them.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed**, and the MCP HTTP token with it if you
  host the server for a remote agent (see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Microsoft Graph API, and you can read every line of how it does so. The skill is
read-only against your tenant and scoped to your own account.
