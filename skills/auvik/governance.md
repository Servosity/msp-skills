# auvik skill - governance and safety model

> Unofficial. Community-built skill for the Auvik API. Not affiliated with,
> endorsed by, or sponsored by Auvik Networks Inc.
> This page tells an MSP owner exactly what the auvik skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `auvik-cli` binary (and `auvik-mcp`), authenticating with
**HTTP Basic**: `AUVIK_USERNAME` (your Auvik user email) plus `AUVIK_API_KEY`.
Both are required - Auvik has no single-token form. `AUVIK_BASE_URL` selects your
region's host (`us1`, `us2`, `eu1`, ...); the wrong region is a 401 that looks
like a bad key. The credential is never logged and never sent anywhere except the
Auvik API.

**Where the credential lives is your choice, and one of the two options writes it
to disk.** Exporting `AUVIK_USERNAME` / `AUVIK_API_KEY` keeps it out of the
filesystem, and that is the recommended path for an agent. `auvik-cli
auth set-credentials <email> <api-key>` instead saves it to the CLI's credentials
file (run `auvik-cli doctor` for the exact path), where it persists until you
replace or delete it. If you use `auth set-credentials`, that file is a secret and
belongs under the same protection as any other stored credential.

An Auvik API key inherits the permissions of the user who created it, so minting
the key as a read-only user is the single most effective control you have.

## Default-safe behavior

- **`--dry-run` is opt-in, and it only guards API writes.** `alert dismiss` (the
  one Auvik-API write) sends immediately unless you pass `--dry-run`, which
  prints the exact request and dials nothing. Make your agent's policy for it:
  preview, show the command, get approval, then run the write.
- **`--dry-run` does NOT guard the LOCAL credential and profile writes.**
  `auth set-credentials ... --dry-run` still writes the credential to
  `credentials.toml`, `auth logout --dry-run` still clears it, and
  `profile save --dry-run` still writes `profiles.json`. These commands touch
  your machine rather than the Auvik API, and the flag does not reach them.
  Treat them as unguarded writes and keep a human on them; do not rely on a
  preview step that does not exist.
- **Read commands are always safe to run** (reports, rollups, search, and all eight
  cross-client analysis commands); they cannot change anything. The eight analysis
  commands read the LOCAL mirror, so they do not even reach the API.
- **`sync` is read-only against the API.** It issues GET requests and writes only to
  the local SQLite mirror on your own machine.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `eol`, `configuration audit`, `inventory diff`, `usage reconcile`, `device discovery-gaps`, `alert noise`, `asm shadow`, `changes`, `sync`, `search`, `export`, every `list` / `get`, and **all of the `settings` and `stat` SNMP-poller commands** - every one of those is a GET despite reading like a setter | Allow |
| **Write (routine)** | Day-to-day mutations. | `alert dismiss-single` and its friendly twin `alert dismiss` - both call `POST /v1/alert/dismiss/{id}`, the ONLY write the Auvik API supports. Allowlist BOTH names | Preview with `--dry-run`, then an approved write |
| **Data egress** | Sends data off this machine to a destination you choose. | `--deliver webhook:<url>` on any command (POSTs that command's output to a URL you name), `feedback --send`, and **bare `feedback` when `AUVIK_FEEDBACK_ENDPOINT` is set and `AUVIK_FEEDBACK_AUTO_SEND=true`** - that pair needs no flag at all, so allowlisting `feedback` as local-only is not safe once both are set (with no endpoint set, `feedback` only writes a local file) | Human-in-the-loop. Read output is still your clients' data; a webhook sink moves it off-box |
| **Credential / security** | Touches tokens or keys. | `auth set-credentials` (writes the credential to the CLI's credentials file), `auth logout` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | (none - the Auvik API exposes no delete) | n/a |
| **Admin** | Back-office administration. | (none - the Auvik API exposes no administrative write) | n/a |

## How to lock it down

- **Scope the credential** to only what your workflow needs. An Auvik key inherits
  the permissions of the user who created it, so a read/report workflow should use
  a key minted by a read-only user should make even `alert dismiss` unreachable - confirm that against your own tenant rather than taking it from us; we have no receipt for Auvik's role model.
  Note what key scope canNOT gate: the Credential tier here is `auth
  set-credentials` / `auth logout`, which write and clear a file on your own
  machine, and the Data egress tier sends to a destination the CALLER names. Those
  are gated by your agent's policy, not by Auvik.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults. Remember `--dry-run` only previews the Auvik API
  write; it does not guard the local credential and profile writes.
- **Never let an agent run Credential or Data-egress tier commands unattended.**
  (This API exposes no delete and no administrative write, so there is no
  Destructive or Admin tier to gate.)
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Auvik API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
