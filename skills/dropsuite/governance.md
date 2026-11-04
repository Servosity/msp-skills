# dropsuite skill - governance and safety model

> Unofficial. Community-built skill for the Dropsuite (NinjaOne) API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the dropsuite skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `dropsuite-cli` binary (and `dropsuite-mcp`), authenticating
with the two values from Settings > API Settings in the Dropsuite partner portal:
`DROPSUITE_RESELLER_TOKEN` (sent as X-Reseller-Token) and `DROPSUITE_ACCESS_TOKEN`
(your reseller Authentication Token, sent as X-Access-Token). `DROPSUITE_HOST` (not a
secret) picks the partner API host. The tokens come from the environment, the config
file, or the 0600 `credentials.toml` that `auth set-token` writes. Each client
organization's own token is read from the organization list at run time (`--org`, or
`DROPSUITE_ORG` for the MCP server) and is never saved.

Every token the program holds, both reseller tokens and each organization token, is
withheld from all output, files and the local store in any encoding. Requests go only
to the configured API host; a redirect to any other host is refused. API responses are
never cached on disk.

## Default-safe behavior

- **The CLI reads.** No command changes a backup, a mailbox, a seat or a setting, and
  no command restores data. `restore-test record` writes to a local ledger on your
  machine only.
- **Sign-in links are the one sensitive action.** `portal get-login-link`,
  `organizations get-support-sso-link`, `locate --sso-link` and `evidence
  --with-sso-link` mint a portal sign-in URL, which is a login credential; a new
  organization link invalidates the previous one. Over MCP these return a preview and
  mint only when the agent repeats the call with `confirm: true`.
- **MCP read-only mode.** `DROPSUITE_MCP_READ_ONLY=1` removes the sign-in link tools and
  the local writers; every read stays.
- **Agent mode is explicit.** `--agent` produces JSON for scripting and adds no gating
  of its own. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **Read plus Local records**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, fleet rollups, search. No change anywhere. | `health`, `coverage`, `billing`, `hygiene`, `locate`, `drift`, `evidence`, `organizations list`, `accounts list`, `onedrives`, `sharepoint-sites`, `teams` | Allow |
| **Local records** | Writes only on your machine; nothing is sent to Dropsuite. | `restore-test record`, `sync`, `export`, `profile save`, `teach` | Allow; review what an agent records |
| **Credential** | Mints a portal sign-in link, or saves a token. Any command that can emit a sign-in URL or a token belongs here. | `portal get-login-link`, `organizations get-support-sso-link`, `locate --sso-link`, `evidence --with-sso-link`, `auth set-token` | Human-in-the-loop only |
| **Destructive (local)** | Erases data this CLI keeps on disk. | `local-data purge` (needs `--yes`), `learnings forget`, `auth logout` | Human-in-the-loop only, explicit confirmation |

## How to lock it down

- **Run the MCP server read-only** for any agent that does not need sign-in links:
  `DROPSUITE_MCP_READ_ONLY=1`.
- **Keep sign-in links human-approved.** They log someone into a client's portal and
  replace the previous link.
- **Rotate the reseller tokens if they are ever exposed**, in Settings > API Settings.
- **Keep HTTP mode behind its token.** `dropsuite-mcp --transport http` refuses to start
  without `DROPSUITE_MCP_HTTP_TOKEN`, checks it on every request, and needs `--tls-cert`/`--tls-key`
  for any address other than loopback.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Dropsuite (NinjaOne) API, and you can read every line of how it does so. The skill is
read-only apart from the sign-in links you approve, and scoped to your own reseller account.
