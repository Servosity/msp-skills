# hook-security skill - governance and safety model

> Unofficial. Community-built skill for the Hook Security API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the hook-security skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `hook-security-cli` binary (and `hook-security-mcp`) with a Hook API
key that an admin creates under **Settings > API keys** in Hook. It is sent as
`Authorization: Bearer <key>` in `HOOK_SECURITY_API_KEY`.

**The key's coverage is the real boundary.** An organization key reads that organization
only. An MSP key reads the MSP and every client under it, including clients added later.
Hook suggests one key per person, so you can revoke one without affecting anyone else.

Where the key lives is your choice:

- **Supplied through the environment** (a secret manager or a session variable): the key
  is never copied to disk.
- **Saved with `auth set-token`** (piped on stdin, so it never appears in process
  arguments or shell history): it is written to a permission-restricted
  `credentials.toml` in your user data directory. Run `hook-security-cli doctor` to see
  the resolved paths, and `auth logout` to clear it.

The key goes only to the API base URL. Two commands send data elsewhere, and only when you
ask: `--deliver webhook:<url>` posts a command's output to that URL (the MCP server does
not accept `--deliver`), and `feedback --send` posts your note to
`HOOK_SECURITY_FEEDBACK_ENDPOINT`, which is unset by default.

## What it can change

**Nothing in Hook.** Hook's API is read-only: every endpoint is a GET, so no command or MCP
tool can create, change or delete a campaign, enrollment, person or setting.

Some commands change files on your own computer: `sync` and `export` write data locally,
`teach`, `teach-lookup`, `teach-pattern`, `teach-playbook`, `learnings`, `playbook` and
`workflow` keep local notes, and `local-data purge` deletes the local copy (a dry run
unless you pass `--yes`).

## What is stored locally

Read responses are kept in a short-lived response cache, and `sync` builds a local SQLite
copy. **Both can hold people's names, email addresses and phishing results, and neither is
encrypted**: they rely on your operating system account and disk encryption.

- Set `HOOK_SECURITY_NO_LOCAL_DATA=1` to keep no local copy: reads go to the API, the
  response cache is off, and `sync` is refused.
- Run `hook-security-cli local-data purge --yes` to erase what is there.

## What your AI sees

Whatever a command returns goes to the AI you use. `clicked-untrained` and
`overdue-training` return people's names and emails, and being on a "who clicked" list is
sensitive for the person named. Use `overdue-training --counts-only` when you only need
per-client numbers, and check that your AI provider's data terms suit employee data.

## Permission tiers

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports and lookups. No change anywhere. | `client-risk`, `clicked-untrained`, `overdue-training`, `qbr`, `organizations`, `campaigns`, `enrollments`, `people`, `groups`, `courses`, `billing`, `search`, `sql` | Allow |
| **Local files only** | Changes files on your computer, never Hook. | `sync`, `export`, the `teach` commands, `local-data purge` | Allow `sync` and `export`; a person runs `local-data purge --yes` |
| **Credential** | Saves or clears the API key. | `auth set-token`, `auth logout` | Human-in-the-loop only |

There is no write, destructive or admin tier because the API offers none.

## How to lock it down

- **Use an organization key** when the workflow covers one client.
- **Keep `HOOK_SECURITY_MCP_READ_ONLY=1` on the MCP server.** It is preset in the one-click
  install and removes the tools that write local files, leaving 31 read-only tools.
- **Rotate the key if it is ever exposed** (for example after bridging the MCP server to a
  public endpoint - see mcp-install.md). Revoke it in Hook under Settings > API keys.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the key, the binary uses it against the Hook API,
and you can read every line of how it does so.
