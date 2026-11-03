# hudu skill - governance and safety model

> Unofficial. Community-built skill for the Hudu API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the hudu skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `hudu-cli` binary (and `hudu-mcp`),
authenticating with a Hudu API key. The key is read from `HUDU_API_KEY`, or from
`credentials.toml` in the CLI's data directory (owner-only permissions) when the
operator saves it with `auth set-token`. It is never logged and never sent anywhere
except the Hudu API.

## Default-safe behavior

- **Vault secrets never leave the client.** Every password-vault response
  (`asset-passwords` list, get, create and update) keeps only metadata (name,
  username, URL, folder, dates), and every asset response replaces the value of
  each filled password-type field (Password, ConfidentialText, or any field type
  the CLI does not know) with `[withheld: password field]`. The password, the OTP
  secret and those field values are never shown, stored or cached, so `search`,
  `sync`, the MCP read tool and MCP `sql` cannot return them, and an error from a
  vault or asset request carries only its HTTP status, never Hudu's error text. A
  request whose body still carries the placeholder is refused, so it is never
  written back over the real value. Fetch a secret in the Hudu portal. A mirror
  written by an older version is scrubbed the first time this version opens it,
  an older cached response is never served, and vault and asset responses are
  never cached.
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
| **Read** | Reports, rollups, search. No change. | `audit completeness`, `audit stale-passwords`, `audit expirations`, `audit summary`, `audit stale-articles`, `audit layout-drift`, `search`, `resolve`, `reconcile`, `companies list`, `assets list-by-company`, `sync`, `doctor`, and any other non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations (reversible archive included). | `articles create`, `articles update`, `assets create`, `assets update`, `asset-layouts create`, `asset-layouts update`, `companies create`, `companies update`, `onboard --apply`, `import`, ... (48 total) | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Reads or writes password-vault entries; manages the API credential. | `asset-passwords get`, `asset-passwords list`, `asset-passwords create`, `asset-passwords update`, `asset-passwords delete`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible data loss. | `articles delete`, `assets delete`, `companies delete`, `flag-types delete`, `flags delete`, `ip-addresses delete`, `lists delete`, `networks delete`, ... (19 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

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
the Hudu API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
