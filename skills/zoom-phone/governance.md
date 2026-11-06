# zoom-phone skill - governance and safety model

> Unofficial. Community-built skill for the Zoom API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the zoom-phone skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `zoom-phone-cli` binary (and `zoom-phone-mcp`) with a Zoom
Server-to-Server OAuth app: `ZOOM_PHONE_ACCOUNT_ID`, `ZOOM_PHONE_CLIENT_ID` and
`ZOOM_PHONE_CLIENT_SECRET`. It mints a one-hour token from Zoom's token endpoint
and sends it only to the Zoom API host. The credentials are saved even when
you set only the environment variables: `auth login` and the first command that
mints a token write the account ID, client ID, client secret and current token
to `credentials.toml` in the data directory, readable only by your user (mode
0600 on macOS and Linux). `auth logout` removes them. `accounts connect` instead
saves the account ID, client ID and client secret in that account's own private
`config.toml`, creates no token and removes any cached `credentials.toml`;
`accounts remove` deletes that account's directory. Reads are saved too: in the default `--data-source auto` mode
every successful read goes into the local SQLite store, and GET responses are
cached for five minutes; `zoom-phone-cli doctor` prints every path, and
`ZOOM_PHONE_NO_LOCAL_DATA=1` turns the store and the cache off. Everything the
program prints, writes to a local file or returns over MCP passes the shared
credential kit: a unit that carries a credential the process holds, in any decoded
form, is withheld whole and replaced by a notice. The client ID and secret go only
to the configured API origin and Zoom's token hosts (`zoom.us`, `zoomgov.com`), so a
`ZOOM_PHONE_TOKEN_URL` anywhere else is refused.

The app can do only what its scopes allow. A read-and-report workflow needs only
the phone admin read scopes; add write scopes only for the changes you want an
agent to make.

## Default-safe behavior

- **In the CLI, `--dry-run` is opt-in - use it.** A CLI write command sends its
  request as soon as it runs; only `--dry-run` prints the request instead, and
  `--yes` does not gate API writes. Make your agent's policy: preview, show the
  exact command, get approval, then run the write.
- **`offboard` plans by default.** It prints the removal plan; only `--apply`
  sends it, after re-checking the user and every group live, and it refuses when
  discovery is incomplete. From the CLI, `--apply` sends the removals with no
  further prompt.
- **`fleet` runs only read commands by default.** A command that writes to a
  client account or returns a secret needs `--allow-writes`, and `offboard`
  through fleet needs exactly one named account.
- **MCP writes need `confirm: true`, always.** Unlike the CLI's opt-in
  `--dry-run`, this gate is not optional: without `confirm: true` the MCP server
  returns the plan and sends no change. A tool that needs live data to build its
  plan, such as `offboard`, still makes the reads for it.
  Setting `ZOOM_PHONE_MCP_READ_ONLY=1` makes the MCP server refuse every write and every secret-returning read.
- **Secret-returning reads are separated.** Reads that can return a device SIP
  password, a PIN or an activation code are not marked read-only for MCP.
- **Agent mode is explicit.** `--agent` produces JSON but adds no write gating -
  the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The tiers come from each command's HTTP method, not its name. The safe default
for an autonomous agent is **read plus previewed writes**; require a human for
anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Audits, traces, reports, search, every GET without a stored secret. No change to Zoom. | `e911 audit`, `numbers inventory`, `devices health`, `callbacks`, `routing trace`, `routing audit`, `footprint`, `offboard` (plan only), `sync`, `search`, `fleet -- <read command>` | Allow |
| **Write (routine)** | POST, PATCH and PUT changes to Zoom Phone configuration. | `users add-call-forward-setting`, call queue member and setting changes, site and common-area updates, outbound caller ID and calling rules | Preview with `--dry-run`, then an approved write |
| **Destructive** | Every DELETE (83 commands) and bulk removal. | `call-queues delete-acall-queue`, `users delete-call-forward-setting`, `offboard --apply`, `fleet --allow-writes` | Human-in-the-loop only, explicit confirmation |
| **Endpoint, messaging and E911** | Changes that reach a desk phone, send a message to an outside party, or change 911 dispatch. | `devices reboot-phone`, `firmware-update-rules` and `provision-templates` changes, `devices update-provision-template-to`, `sms post-message`, `whatsapp post-whats-app-message`, `emergency-addresses` and `emergency-services` changes | Human-in-the-loop only |
| **Credential / identity** | Any command that can return a stored secret field (SIP password, PIN, activation code, channel token), role and membership changes, and the commands that store credentials. | `devices get-adevice`, `common-areas list-activation-codes`, `users phone-settings`, `setting-templates get`, `ported-numbers get-details`, `alert-settings get-details`, `roles` add/update/delete, `auth`, `accounts connect` | Human-in-the-loop only |

## How to lock it down

- **Scope the Zoom app** to what your workflow needs. Read-only phone admin
  scopes make every write tier fail at Zoom, whatever the agent tries.
- **Run the MCP server with `ZOOM_PHONE_MCP_READ_ONLY=1`** for agents that only
  report.
- **Keep autonomous agents to Read + previewed writes.** The approval gate for
  CLI writes lives in your agent's policy, not in the binary's defaults.
- **Never let an agent run Destructive, Endpoint or Credential tier commands
  unattended.**
- **Use one profile per client** (`accounts connect`), so a command can only act
  on the account you selected.
- **Rotate the Client Secret if it is ever exposed** (for example after hosting
  the MCP server over HTTPS for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Zoom API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
