# riverside-fm skill - governance and safety model

> Unofficial. Community-built skill for the Riverside API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the riverside-fm skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `riverside-fm-cli` binary (and `riverside-fm-mcp`). There
is no API key to hand it: this connector authenticates as **you**, reusing the
session your own browser already holds. `auth login` reads the matching cookies
out of your local browser cookie store and writes them to your own config file
(mode 0600, path shown by `doctor`). Those cookies are sent to the Riverside API
and nowhere else. They carry your full account access, so treat that config file
exactly like a password.

## Default-safe behavior

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
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `import` (POSTs records from a local JSONL via the API's create/upsert path) is the only mutating command; the verb-named capability checks (`ai can-create-event`, `clips get-patches`, `takes get-assets`, `takes get-clip-assets`) are read-only GETs the scanner flagged by name | Preview with `--dry-run`, then an approved write (where a command documents its own confirm gate, use it too) |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | (none detected) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |
| **Local browser session** | Reads your own browser's cookie store on this machine to obtain a session, and saves that session to your local config file. Reaches outside the vendor API into your workstation. It can also attach to a browser you already have running and read `document.cookie` for the vendor domain. | `auth login`, `doctor` | Operator runs it, once, interactively. Never leave it to an unattended agent. |

## Local browser session: what "cookie auth" actually means here

Riverside publishes no partner API key, so this connector authenticates as you by
reusing your browser's own session. That is a different kind of access from an
API key in an environment variable, and it deserves its own line in the table
above rather than being filed under Credential / security.

**What it reads.** `auth login --chrome` looks in the standard browser profile
location for your operating system and finds the profile that holds cookies for
the vendor domain. Only that domain's cookies are extracted and saved, through a
cookie-extraction helper you install yourself; the connector never implements
decryption of your cookie store itself. It can also attach to a browser you
already have running and read `document.cookie` for the vendor domain.

**One thing worth knowing about the profile probe.** To work out WHICH of your
browser profiles is signed in, the connector copies each profile's cookie
database to a temporary file and counts the rows matching the vendor domain.
That copy is the whole database - every site's cookies, not just this vendor's -
because SQLite has to open the file as a unit, and Chrome holds a write lock on
the original. The copy is made inside a 0700 directory with the files at 0600,
it is deleted as soon as the count is taken, and nothing from it is read except
the row count. Nothing is transmitted.

**What it can launch.** The external programs the CLI itself can run are fixed
at build time, and this list is read straight out of the source:
`agent-browser`, `browser-use`, `cookie-scoop`, `cookies`, `python3`, `sqlite3`.
Every one is a compile-time literal - no command name is ever built from your
input - and the connector never invokes a shell. One exception, and it is worth
knowing about: the MCP server does not call the vendor API itself, it re-invokes
the companion CLI, and it finds that binary next to itself, then at
`RIVERSIDE_FM_CLI_PATH`, then on PATH. Anyone who can set that variable in the
MCP server's environment chooses which binary runs. Treat it like any other
entry in that environment: if an attacker can already set it, they can already
run code as you, but do not let an untrusted process supply it.

**What leaves the machine.** The extracted cookies are written to your own
config file at mode 0600 and are sent as request headers to the vendor API.
Nothing is sent anywhere else, and nothing is logged.

**What an MSP owner should require.**

- Run `auth login` yourself, interactively, on your own workstation. It is not
  an agent operation.
- Treat the config file as a password store: those cookies are full account
  access until the vendor session expires.
- Prefer a dedicated browser profile signed in to only this vendor if you share
  the workstation.
- If you ever ran it somewhere you should not have, sign out of the vendor in
  that browser to invalidate the session, then delete the config file.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md). For this
  connector rotating means signing out of Riverside in the browser you logged
  in with, then deleting the config file and running `auth login` again.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Riverside API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
