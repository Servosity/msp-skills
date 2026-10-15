# quotewerks skill - governance and safety model

> Unofficial. Community-built skill for the QuoteWerks Web REST API. Not affiliated
> with, endorsed by, or sponsored by Aspire Technologies, Inc.
> This page tells an MSP owner exactly what the quotewerks skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `quotewerks-cli` binary (and `quotewerks-mcp`), which sends a
QuoteWerks Web API key as the `X-API-Key` header to `https://qwwapi.quotewerks.com`.
You create the key in QuoteWerks Web under **Setup > Options > Zapier and API**; the
API requires a user with MasterRights. The key is read from `QUOTEWERKS_API_KEY`.

It is written to disk only if you choose to save it with
`echo "$QUOTEWERKS_API_KEY" | quotewerks-cli auth set-token`, which stores it in the
CLI's credentials file with mode 0600; `quotewerks-cli auth logout` removes it.
`--dry-run` output masks the key. It is never sent anywhere except the QuoteWerks Web API.

The local mirror (`quotewerks-cli mirror`) stores copies of your quotes, line items
and product databases in a SQLite file under your user account. Delete the CLI's
data directory to remove it. Nothing else from your tenant is written to disk unless
you ask for it with `export`: API responses are never cached, rows that `tables` reads
or writes are never copied into the local database, and `sync` stores only the list of
table names (it refuses `cloud_settings`, whose rows can hold integration secrets).

## Default-safe behavior

- **Typed writes preview by default.** `docs create`, `items add`, `items reprice` and
  `requote` print the exact request (and, for repricing, old versus new cost, price and
  margin per line) and write nothing until you add `--confirm`. `--yes` does not
  confirm them.
- **Raw writes send immediately.** `tables insert`, `tables update` and `tables delete`
  are thin wrappers over the vendor's table endpoints. Run them with `--dry-run` first
  to see the HTTP request without sending it. (`import` accepts only `agent`: it sends
  each line of a JSONL file to the QuoteWerks Web AI agent, like `agent ask`.)
- **Read commands are always safe to run** (lookups, audits, reports); they cannot
  change anything in QuoteWerks. `mirror` and `sync` write only the local copy.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating and does not imply `--confirm`. See AGENTS.md.
- **The MCP server cannot read local files you did not choose.** File-path flags
  (`--db`, `--input`, `--notes-file`, `--playbook-file`, output paths) are refused
  over MCP, and `import` is CLI-only.
- **Raw table writes are CLI-only.** `tables insert`, `tables update` and `tables delete`
  are not offered as MCP tools; over MCP an agent can only change quotes through the
  previewed typed writes.

## Permission tiers

The safe default for an autonomous agent is **reads plus previewed writes**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Lookups, audits, reports. No change in QuoteWerks. | `docs search`, `docs get`, `docs lint`, `items list`, `products search`, the `tables` reads (`list`, `rows`, `row`, `query`, `schema`, `values`), the `cloud-settings` reads, `reports run`, `reports schedulability`, `identity userinfo`, `agent ask` (and `import agent`, CLI only), the `agent report-*` previews, and the mirror reports (`margin`, `pipeline`, `cost-drift`, `winrate`, `changes`), `search`, and `export` (reads from QuoteWerks into a local file); `mirror` and `sync` write only the local copy | Allow |
| **Previewed writes** | Create or change quotes; preview unless `--confirm`. | `docs create`, `items add`, `items reprice`, `requote` | Let the agent preview; a human approves before it re-runs with `--confirm` |
| **Raw writes** | Create or change any row the key can write, immediately. CLI only, never an MCP tool. | `tables insert`, `tables update` | Preview with `--dry-run`, then an approved write |
| **Destructive** | Irreversible data loss. CLI only, never an MCP tool. | `tables delete` (any table, documents and line items included) | Human-in-the-loop only, explicit confirmation |
| **Credential** | Stores or removes the API key on this machine. | `auth set-token`, `auth logout` | Human-in-the-loop only |

`agent ask` sends your question to QuoteWerks Web's own AI agent, which answers from
your QuoteWerks data; treat it like any other read that leaves your machine.

## How to lock it down

- **Leave Allow write access off** unless the workflow needs writes. A read-only key
  covers every lookup, audit and report, and QuoteWerks answers any write with 403.
- **Run the MCP server read-only** for agents that only answer questions: set
  `QUOTEWERKS_MCP_READ_ONLY=1` (exactly `1`). It drops sync, workflow archive, export,
  reports run, agent ask, agent report-draft, learnings forget and learnings reject, and
  refuses `--confirm` on the typed writes, which can still preview. Local-only tools such
  as `mirror` and `teach` stay.
- **Use a separate key for the agent** so you can revoke it in Setup > Options >
  Zapier and API without touching other integrations.
- **Keep autonomous agents to reads and previews.** Have a human approve every
  `--confirm` and every raw `tables` write.
- **Never let an agent run the Destructive or Credential tiers unattended.**
- **Rotate the key if it is ever exposed** (for example after bridging the MCP server
  to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the key, the binary uses it against the
QuoteWerks Web API, and you can read every line of how it does so. The skill is
read-first, previews its typed writes, and can never do more than the key you create.
