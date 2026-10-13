# granola skill - governance and safety model

> Unofficial. Community-built skill for the Granola API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the granola skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `granola-cli` binary (and `granola-mcp`),
authenticating with `GRANOLA_API_KEY`, read from the environment or from the CLI's
credentials file (written owner-only by `auth set-token`, which takes the key on stdin,
never as an argument). The optional `mcp-auth login` stores Granola's official-MCP
OAuth tokens in the macOS Keychain. Credentials are never logged and never sent
anywhere except Granola.

## Default-safe behavior

- **Nothing changes your Granola account.** The CLI has no command that creates,
  edits, or deletes Granola data. Its writes are local: the SQLite mirror (`sync`,
  `enrich`, `sync-api`), markdown files on disk (`export`, `export-all`, `extract`,
  `memo run`, `collect`), the saved API key (`auth set-token`), and the Granola-MCP
  sign-in (`mcp-auth login`).
- **Two commands can send output off the machine, and only when you ask.** Any
  command accepts `--deliver webhook:<url>`, which POSTs its output to that URL, and
  `feedback --send` posts a feedback note to `GRANOLA_FEEDBACK_ENDPOINT`. The MCP
  server refuses `--deliver`; an agent driving the CLI directly does not, so keep
  `--deliver` out of an unattended agent's allowed commands.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus local writes to folders you
choose**; require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `search`, `meetings list/get`, `transcript get`, `attendee timeline/brief`, `talktime`, `stats`, `doctor` | Allow |
| **Local writes** | Writes the local mirror or files on your own machine. Never your Granola account. | `sync`, `enrich`, `sync-api`, `workflow archive`, `export` / `export-all`, `extract`, `memo run`, `collect` | Allow, with output folders you choose |
| **Credential / security** | Touches tokens and stored credentials. | `auth set-token`, `auth logout`, `mcp-auth login`, `mcp-auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | (none - no command changes or deletes Granola data) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

Note: the local learning and profile commands (`teach*`, `playbook`, `learnings`,
`profile`) write only to the local store/config on your own machine - they never
change your Granola account.

## How to lock it down

- **Scope the credential** to only what your workflow needs: a Personal API key sees
  only your own notes; a Workspace key sees workspace-visible notes.
- **Keep autonomous agents to Read + Local writes.** Choose the output folders for
  exports yourself. The MCP server drops any file or folder path a tool call supplies
  (`--out`, `--to`, `--root`, `--db` and the like), so over MCP a command runs with its
  default location; the file-writing commands are meant for the CLI.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Granola API, and you can read every line of how it does so. The skill is
read-only against Granola and scoped to your own account.
