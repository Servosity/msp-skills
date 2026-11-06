# comet-backup skill - governance and safety model

> Unofficial. Community-built skill for the Comet Backup API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the comet-backup skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `comet-backup-cli` binary (and `comet-backup-mcp`) against one
Comet Server, named by `COMET_BACKUP_SERVER` (host and port) or `COMET_BACKUP_BASE_URL`
(full URL). It signs in as a Comet admin with `COMET_BACKUP_USERNAME` plus
`COMET_BACKUP_PASSWORD`, or with a session key in `COMET_BACKUP_SESSION_KEY`. Credentials
come from the environment or the CLI's owner-only `credentials.toml`; they are sent only
to that server, and a redirect to any other host is refused. Credential fields in Comet
responses (password hashes, TOTP keys, storage keys) are printed as `[redacted]` and are
never written to the local store; there is no response cache.

## Default-safe behavior

- **Reads change nothing.** `health`, `triage`, `usage`, `devices stale`, the job
  history and the server status commands only read.
- **CLI writes send when you run them.** Use `--dry-run` first to see the request
  without sending it. `rerun-failed` is the exception: it prints its plan and
  dispatches only with `--yes`.
- **MCP writes need `confirm: true`.** Without it the MCP server returns a preview and
  sends nothing. The ten credential-tier reads below need `confirm: true` too.
- **`COMET_BACKUP_MCP_READ_ONLY=1`** removes every write tool from the MCP server and
  refuses the credential-tier reads.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but adds no write
  gating. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **Read plus previewed writes**; require a
human for every tier below Write (routine). Every Comet admin call is a POST, so these
tiers come from what each command does, not from its HTTP method.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, lists. No change. | `health`, `triage`, `usage`, `devices stale`, `devices outdated`, `devices online`, `backup-jobs recent`, `server version`, `users list`, `policies list` | Allow |
| **Credential** | Reads that return credential material from Comet (redacted by the CLI). | `users get`, `users list-full`, `user-groups get`, `organizations`, `storage buckets`, `storage bucket`, `constellation last-report`, `policies get`, `policies list-full`, `protected-items` | Human-in-the-loop only |
| **Write (routine)** | Day-to-day changes on the server. | `rerun-failed --yes`, `devices run-backup`, `devices run-backup-custom`, `backup-jobs cancel`, `users add`, `users set-profile`, `storage request-vault`, `import` | Preview with `--dry-run`, then an approved write |
| **Customer endpoint** | Runs an action on a customer's device. | `devices run-restore`, `devices update-software`, `devices uninstall`, `devices unlock`, `devices kill`, `devices disconnect`, `devices apply-retention`, `devices reindex`, `devices deep-verify` | Human-in-the-loop only |
| **Destructive / security** | Irreversible loss, or account security. | `users delete`, `users reset-password`, `users disable-totp`, `users revoke-device`, `backup-jobs abandon` | Human-in-the-loop only, explicit confirmation |
| **Local only** | Writes only this machine's store. | `snapshot`, `changes`, `local-data purge --yes` | Allow |

## How to lock it down

- **Use a dedicated Comet admin** with only the rights your workflow needs. A
  reporting workflow does not need an admin that can delete users or uninstall
  devices.
- **Run the MCP server with `COMET_BACKUP_MCP_READ_ONLY=1`** for reporting agents.
- **Keep autonomous agents to Read plus previewed writes.** Have a human approve each
  write; the CLI does not prompt before sending.
- **Never let an agent run Credential, Customer endpoint or Destructive tier commands
  unattended.**
- **Rotate the password if it is ever exposed** (for example after bridging the MCP
  server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Comet Backup API, and you can read every line of how it does so. The skill is
read-first and scoped to the Comet Server and admin account you name.
