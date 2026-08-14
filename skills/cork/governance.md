# cork skill - governance and safety model

> Unofficial. Community-built skill for the Cork API. Not affiliated with,
> endorsed by, or sponsored by Cork.
> This page tells an MSP owner exactly what the cork skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `cork-cli` binary (and `cork-mcp`), authenticating with
`CORK_API_KEY` as a bearer token. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the Cork API.

One property of the Cork API is worth knowing before you mint a key: **a Cork API
key inherits the permissions of the user who created it.** A key minted by an
operator without distributor scope returns 403 on the distributor endpoints; that
is the credential's scope, not a broken command. This is the single most effective
control you have, so use it: mint the key as a user whose permissions match the
work you want the agent to do.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you
  pass `--dry-run` first to preview the exact request without sending it. Make your
  agent's policy: preview, show the command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search, and all eight
  cross-client analysis commands); they cannot change anything.
- **`sync` is read-only against the API.** It issues GET requests and writes only to
  the local SQLite mirror on your own machine.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See
  [AGENTS.md](./AGENTS.md).

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search, and cross-client analysis. No change. | `score attribute`, `score regressions`, `vulnerabilities triage`, `vulnerabilities exposure`, `compliance overdue`, `integrations health`, `coverage gaps`, `warranties exposure`, `sync`, `search`, `export`, and every `list` / `get` command | Allow |
| **Write (routine)** | Day-to-day mutations against integration config. | `integrations connect`, `integrations update`, `integrations resync integration` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Reads or replaces stored connector credentials. | `integrations credentials`, `integrations credentials get-integration`, the credential fields of `integrations update` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `integrations delete` (stops all data collection from that connector) | Human-in-the-loop only, explicit confirmation |
| **Endpoint-affecting** | Changes the state of a real customer machine. | `software install` (installs a package on a mapped device through that device's RMM integration) | Human-in-the-loop only, explicit confirmation. Never unattended. |
| **Admin** | Back-office administration of the partner hierarchy. | `distributor provision-partner` (creates a new Partner account; requires distributor privileges) | Operator-only, not for agents |

Two rows above deserve emphasis because they reach outside Cork itself:

- **`software install` installs software on a real customer endpoint.** It is not a
  Cork-internal record change. Treat it exactly as you would treat a manual RMM
  push, with the same approval and change-control you already require there.
- **`distributor provision-partner` creates a partner account** in the Cork
  hierarchy. It is a commercial and structural change, not an operational one.

## How to lock it down

- **Scope the credential to only what your workflow needs.** Because a Cork key
  inherits its creator's permissions, a read-and-report workflow should use a key
  minted by a read-only user. That single choice makes the Destructive,
  Endpoint-affecting, and Admin tiers unreachable regardless of what an agent asks
  for.
- **Keep autonomous agents to Read plus previewed writes.** Have a human approve the
  actual write for Write tier and above; the gate lives in your agent's policy, not
  in the binary's defaults.
- **Never let an agent run Credential, Destructive, Endpoint-affecting, or Admin
  tier commands unattended.**
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see [mcp-install.md](./mcp-install.md)).

## Where your data goes

The offline mirror is a SQLite database under your own user account (run
`cork-cli doctor` to see the exact path). Client names, device identifiers,
compliance events, and vulnerability findings for your whole book of business live
in that file once you run `sync`, so treat it with the same care as any other export
of client data: it inherits your disk encryption and your backups, and nothing more.
Your AI agent sees only what a command returns, which is normally a query result
rather than the raw mirror.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Cork API, and you can read every line of how it does so. The MCP server speaks
stdio only - it does not open a network listener - so the whole-fleet read key it
holds is reachable only by the local agent process you started it from. The skill is
read-first, plan-by-default, and scoped to your own account.
