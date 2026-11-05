# apple-business skill - governance and safety model

> Unofficial. Community-built skill for the Apple Business API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the apple-business skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `apple-business-cli` binary (and `apple-business-mcp`),
authenticating as one Apple Business API account: `APPLE_BUSINESS_CLIENT_ID` (the account's Client ID),
`APPLE_BUSINESS_KEY_ID` and `APPLE_BUSINESS_PRIVATE_KEY` (the account's private key, or the path to its .pem file).
The CLI signs a short-lived ES256 client assertion with the key, exchanges it at Apple's token endpoint
(account.apple.com) for a one-hour bearer token, and sends that token only to the Apple Business API
(api-business.apple.com); any other host and any redirect to another host is refused. Credentials come from the
environment, a `--config` file, or `auth login`, which keeps them only in the private credential store (a 0600 file
in the data directory). They are withheld, in any encoding, from every output, log, exported file, local store and
MCP result. An API account belongs to one organization, so each customer org has its own.

## Default-safe behavior

- **Over MCP, every write previews first.** Each write tool returns a preview and runs only when called with
  `confirm: true`, and no MCP argument can point the server at a file on your machine.
- **On the command line, `--dry-run` is opt-in - use it.** Writes send immediately unless you pass `--dry-run`
  to preview the request without sending (`devices assign` and `devices unassign` included). Make your agent's
  policy: preview, show the exact command, get approval, then run the write.
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
| **Read** | Reports, lookups, sync into the local store. No change in Apple Business. | `devices list`, `devices lookup`, `devices unassigned`, `devices locked`, `applecare expiring`, `fleet summary`, `mdm-servers health`, `audit-events digest`, `sync`, `devices enrich` | Allow |
| **Write (routine)** | Creates and changes Blueprints, Configurations and MDM servers. | `blueprints create`, `blueprints update`, `blueprints add-apps`, `configurations create`, `configurations update`, `mdm-servers create`, `mdm-servers update`, `import` | Preview with --dry-run, then a reviewed write |
| **Device assignment** | Changes which MDM server customer devices enroll with. | `devices assign`, `devices unassign`, `device-activities create` | Human-in-the-loop only |
| **Destructive** | Deletes Blueprints, Configurations or MDM servers, removes Blueprint members, erases local data. | `blueprints delete`, `blueprints remove-devices`, `configurations delete`, `mdm-servers delete`, `local-data purge` | Human-in-the-loop only, explicit confirmation |
| **Credential** | Saves, replaces or clears the API account credentials. | `auth login`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Admin** | Back-office administration. | (none: the API has no account or role management) | Operator-only, not for agents |

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
the Apple Business API, and you can read every line of how it does so. The skill is
read-first, previews writes over MCP, and is scoped to the API account you give it.
