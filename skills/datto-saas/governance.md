# datto-saas skill - governance and safety model

> Unofficial. Community-built skill for the Datto SaaS Protection API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the datto-saas skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `datto-saas-cli` binary (and `datto-saas-mcp`), authenticating
with the Datto REST API key pair from the Partner Portal: `DATTO_SAAS_PUBLIC_KEY` and
`DATTO_SAAS_SECRET_KEY`, sent as HTTP Basic auth only to the configured API host (`api.datto.com`, unless
`DATTO_SAAS_BASE_URL` or `base_url` in `config.toml` names another). They are read
from the environment, or from the private (0600) credentials file that
`auth set-credentials` writes; they are never logged and never sent to any other host. The shared credential kit
enforces this: output, files, the local store or a request to another host that would
carry a held credential, in any encoding, is withheld instead.
One partner key sees every client you manage.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they change nothing
  at Datto. A read that runs through the CLI adds a line to the local learning journal unless
  `--no-learn` is set, and `recall` and `playbook list` also add a learning record.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned seat changes**
(`seats bulk-plan` sends nothing); require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change at Datto. | `seats gaps`, `seats reconcile`, `seats summary`, `seats roster`, `backup-health`, `exit-inventory`, `seats bulk-plan` (plans only), `domains`, `seats list`, `applications`, `backup-stats`, `activity-log`, `search`, `analytics` | Allow |
| **Local state** | Writes only on your machine: the local mirror, a file, the learning store. | `sync`, `export`, `workflow archive`, `local-data purge`, `learnings`, `teach*`, `playbook amend`, `profile`, `feedback` | Allow |
| **Seat and billing change** | Licenses, pauses or unlicenses up to 100 seats per call. Changes what is backed up and what you are billed; Unlicense stops protecting the seat. | `seats bulk-change` | Human-in-the-loop only: plan with `seats bulk-plan`, preview with `--dry-run`, then a reviewed send. Over MCP it previews and sends only when `confirm` is boolean `true`. |
| **Credential / config** | Replaces or clears the stored key pair. | `auth set-credentials`, `auth logout` | Human-in-the-loop only |

`DATTO_SAAS_MCP_READ_ONLY=1` makes the MCP server leave out `datto-saas_execute` (the only
path to the seat change), `sync`, `export`, the exit-inventory recipe and the commands that
change local state; the other reads stay available. A read that runs through the CLI still
adds a line to the local learning journal unless `no-learn` is set, and `recall` and
`playbook_list` also add a learning record.

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
the Datto SaaS Protection API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
