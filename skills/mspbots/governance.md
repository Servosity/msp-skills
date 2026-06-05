# mspbots skill - governance and safety model

> Unofficial. Community-built skill for the MSPbots API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the mspbots skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `mspbots-cli` binary (and `mspbots-mcp`), authenticating
with an MSPbots API key read from `MSPBOTS_API_KEY` in the environment or saved
to the CLI's local config file via `mspbots-cli auth set-token`. The key is
never logged and never sent anywhere except the MSPbots API.

## Default-safe behavior

- **The MSPbots Public API is read-only.** It exposes dataset and widget reads
  and nothing else - there is no command in this skill that can create, change,
  or delete anything in your MSPbots tenant.
- **The only writes are local.** `registry add`/`rm` (aliases), `snapshot`
  (point-in-time copies), and `sync` (offline cache) write to a SQLite store
  under your own user account. Deleting that file loses local history and
  aliases; your MSPbots tenant is untouched.
- **Agent mode is explicit.** `--agent` produces JSON for scripting; it does not
  change what the commands can reach. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **everything except credential
changes** - an unusual luxury this API's read-only design makes possible.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read (live API)** | Fetches rows from datasets/widgets bound to your key. No change possible. | `pull`, `export`, `describe`, `dataset`, `widget`, `doctor` | Allow |
| **Local-only writes** | Writes to your local SQLite store; never touches the tenant. | `registry add`, `snapshot`, `sync` | Allow - safe to schedule |
| **Credential / local config** | Saves or clears the stored API key; removes local aliases. | `auth set-token`, `auth logout`, `registry rm` | Human-in-the-loop only |

## How to lock it down

- **The binding is the permission boundary.** An MSPbots admin creates the API
  key at Settings > Public API and explicitly binds each dataset or widget to
  it. The key can only read what is bound - bind only what you want an AI to
  see. The global "Enable Public API" toggle gates everything.
- **Prefer narrow bindings over broad ones.** A QBR workflow needs the ticket
  and SLA datasets, not the finance ones.
- **Rotate the key if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).
- **Treat the local store as data.** Snapshots of your datasets live in the
  local SQLite file; protect it like any other export of your BI rows.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it
against the MSPbots API, and you can read every line of how it does so. The API
is read-only by design, the skill adds no write path, and the blast radius of
any command is bounded by what an admin bound to the key.
