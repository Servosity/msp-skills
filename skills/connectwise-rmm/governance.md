# connectwise-rmm skill - governance and safety model

> Unofficial. Community-built skill for the ConnectWise RMM (Asio) partner API. Not
> affiliated with, endorsed by, or sponsored by ConnectWise, LLC.
> This page tells an MSP owner exactly what the connectwise-rmm skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `connectwise-rmm-cli` binary (and `connectwise-rmm-mcp`) with an
API client you create under **Integrations > API Access** in the ConnectWise portal:
`CONNECTWISE_RMM_CLIENT_ID` and `CONNECTWISE_RMM_CLIENT_SECRET`. The CLI exchanges them
for a short-lived bearer token (OAuth2 client credentials) at the token endpoint of your
region, which the client ID's prefix picks. Two optional settings:
`CONNECTWISE_RMM_BASE_URL` overrides the region's API host, and
`CONNECTWISE_RMM_OAUTH_SCOPE` narrows the scopes requested (run `connectwise-rmm-cli scopes`
to see which ones your key was granted).

**What is stored on disk.** After a successful token request the CLI caches the bearer
token, together with the client ID and secret it was minted from, in a private
`credentials.toml` (file mode 0600) under the CLI's data directory, so later runs can
re-mint without asking again. `connectwise-rmm-cli auth logout` clears all three.
**Removing the environment variables does not revoke access** - the cached copy keeps
working until you run `auth logout` or rotate the client secret in the ConnectWise portal.
Credentials are never sent anywhere except the ConnectWise RMM token and API endpoints.

## Default-safe behavior

- **`run` previews by default.** It resolves the script and the target endpoints and
  prints the exact request, and schedules nothing until you add `--confirm`. Under a
  test harness it refuses to schedule at all.
- **`--dry-run` is opt-in for every other write - use it.** Ticket, site, custom-field,
  task and uninstall commands send immediately unless you pass `--dry-run` to preview
  the request first. Make your agent's policy: preview, show the exact command, get
  approval, then run the write.
- **Read commands are always safe to run** (fleet roll-ups, search, every list and get);
  they cannot change anything in ConnectWise RMM.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previewed writes**; require a
human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, roll-ups, search, lookups. No change in ConnectWise RMM. | `fleet stale`, `fleet patch-posture`, `fleet reboot-due`, `fleet whois`, `fleet brief`, `fleet scorecard`, `fleet task-failures`, `scopes`, `sync`, `search`, `analytics`, `export`, `run` without `--confirm`, every `list` / `get` command (including the bulk `patches get-*` reads) | Allow |
| **Write (records)** | Creates or changes tickets, sites and custom fields. | `tickets create`, `tickets update`, `ticket-notes create`, `sites create`, `sites update`, `custom-fields set-company`, `custom-fields set-site`, `custom-fields set-endpoint` (`set-company` also answers to `custom-fields update`), `import` of ticket records | Preview with `--dry-run`, then an approved write |
| **Endpoint actions** | Runs a script on real client machines. | `run --confirm`, `tasks schedule` (alias `tasks create`), `import` of task records (each line schedules a script run, with no preview) | Human-in-the-loop, explicit confirmation |
| **Destructive** | Removes the agent; the endpoint stops being managed. | `endpoints uninstall` (alias `endpoints delete`) | Human-in-the-loop only, explicit confirmation |
| **Credential** | Issues or stores credentials. | `endpoints create-agent-install-token` (alias `endpoints create`; a token that enrolls new agents into a site), `import` of endpoint records (each line mints an install token), `auth login`, `auth set-token` | Human-in-the-loop only |

**The MCP server.** Besides one tool per fleet command, the MCP server exposes a generic
`connectwise-rmm_execute` tool that can call any partner API operation, including
`tasks.schedule` and `endpoints.uninstall`, with no preview step. An allowlist keyed on
CLI command names does not cover it. Set `CONNECTWISE_RMM_MCP_READ_ONLY=1` in the MCP
server's environment and it drops its write tools (`run`, `import`, `sync` among them)
and refuses every non-read call through `connectwise-rmm_execute`.

## How to lock it down

- **Scope the API client** to only what your workflow needs. A reporting workflow needs
  read scopes only (`platform.companies.read`, `platform.sites.read`,
  `platform.asset.read`, `platform.devices.read`, `platform.patching.read`,
  `platform.tickets.read` for ticket counts, `platform.automation.read` for
  `fleet task-failures`); leave out
  `platform.automation.create` (script runs), `platform.agent.delete` (uninstall),
  `platform.agent-token.read` (install tokens) and the `.write` / `.create` / `.update`
  scopes unless a human-approved workflow needs them.
- **Run the MCP server read-only for reporting agents:** `CONNECTWISE_RMM_MCP_READ_ONLY=1`.
- **Switching API clients takes effect only after `connectwise-rmm-cli auth logout`.** A cached
  client ID and secret win over the environment variables, so narrowing scopes by
  exporting a different key does nothing until the cached one is cleared.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the actual
  write for the Write tier and above - the gate lives in your agent's policy, not in the
  binary's defaults (except `run`, which previews by default).
- **Never let an agent run Endpoint actions, Destructive or Credential tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the client secret if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md), then run
  `connectwise-rmm-cli auth logout` to clear the cached copy.

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the API client, the binary uses it against the
ConnectWise RMM partner API, and you can read every line of how it does so. The skill is
read-first, previews script runs by default, and is limited to the scopes your API key
was granted.
