# prtg skill - governance and safety model

> Unofficial. Community-built skill for the PRTG API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the prtg skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `prtg-cli` binary (and `prtg-mcp`) with PRTG API v2 keys,
sent as `Authorization: Bearer <key>`. For one core it reads `PRTG_SERVER` and
`PRTG_API_KEY`; for the fleet commands, `PRTG_FLEET_KEYS` holds the key of each
core registered with `core add`, as `name=key` pairs. A fleet child process for
one core sees only that core's key.
Keys are read from the environment (or the 0600 credential store written by
`auth set-token`), are never written to the fleet registry, logs or the local
mirror, and are sent only to the configured PRTG core: a redirect or a request to
any other host is refused. Each key acts with the access level it was created
with in PRTG, so a read-only key cannot pause or acknowledge anything.

## Default-safe behavior

- **The fleet actions preview by default.** `fleet ack`, `fleet pause` and
  `fleet resume` show, per core, what they would change and how many objects
  match, and send nothing until you add `--yes`.
- **Single-core writes send when run.** Typed write commands (`sensors pause sensor`,
  `devices update`, `sensors delete`, ...) send immediately unless you pass
  `--dry-run` first. Make your agent's policy: preview, show the exact command, get
  approval, then run the write.
- **MCP writes need confirm.** Every MCP tool or endpoint that changes PRTG
  returns a preview until it is called again with `confirm: true`. Set
  `PRTG_MCP_READ_ONLY=1` and the MCP server refuses every write.
- **Read commands are always safe to run** (fleet views, gets, lists, search,
  analytics); they cannot change anything in PRTG.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating, and does not imply `--yes`. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus previews**; require a
human for anything below the line. Tiers follow each command's HTTP method in
`prtg-cli agent-context`, not its name.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Fleet views, gets, lists, search. No change. | `fleet alarms`, `fleet status`, `fleet handoff`, `fleet silenced`, `fleet drift`, `fleet exec`, `fleet sync`, `sensors get`, `devices list-experimental`, `sync`, `search`, `analytics`, `users find-apikey` | Allow |
| **Write (routine)** | Day-to-day monitoring changes. | `fleet ack`, `fleet pause`, `fleet resume`, `sensors acknowledge sensor`, `sensors multi-acknowledge`, the `pause` / `resume` / `multi-pause` / `multi-resume` / `scan` / `multi-scan-now` commands on sensors, devices, groups and probes, `devices create-sensor`, `groups create-device-for`, `groups create-for`, `probes create-device-for`, `probes create-group-for`, `sensors update`, `devices update`, `groups update`, `probes update`, `objects clone object`, `objects move object`, `autodiscoverytasks modify-auto-discovery-task`, `ai analyze-aiincidents` (a POST that only analyzes, previewed like a write), `import` | Preview first (`--dry-run`; the fleet commands preview by default; MCP `confirm`), then an approved write |
| **Acts on client networks** | Starts discovery or scans on a client's network. | `devices trigger-autodiscovery`, `groups trigger-autodiscovery`, `autodiscoverytasks start-auto-discovery-task`, `probes create-auto-discovery-task`, `devices metascan` | Human-in-the-loop only |
| **Credential / identity** | Any command that can emit a secret, and user and key management. | `session login`, `session renew`, `users api-keys create`, `system license-info` (the license key is withheld from output, but treat the command as credential tier), `users api-keys list`, `users modify-apikey`, `users reset-password`, `users trigger-password-reset`, `users pause user`, `users resume user`, `users multi-pause-request`, `users multi-resume`, `session logout`, `auth set-token`, `auth logout`, `core add`, `core remove` | Human-in-the-loop only |
| **Destructive** | Irreversible deletion in PRTG or of local data. | `sensors delete`, `devices delete`, `groups delete`, `probes delete`, `objects multi-delete`, `autodiscoverytasks delete-auto-discovery-task`, `users delete-apikey`, `local-data purge --yes` | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none: API v2 does not expose server settings) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the PRTG API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
