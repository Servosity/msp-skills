# hubspot skill - governance and safety model

> Unofficial. Community-built skill for the HubSpot API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the hubspot skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `hubspot-cli` binary (and `hubspot-mcp`),
authenticating with `HUBSPOT_ACCESS_TOKEN` from the environment, or with a token you saved
using `hubspot-cli auth set-token` (stored in the CLI's own credentials file under its data
directory). Credentials are never logged and never sent anywhere
except the HubSpot API.

## How writes actually behave (read this before trusting an agent)

- **Every command that changes your HubSpot portal needs `--yes` or `--dry-run`.**
  Each create, update, upsert, merge, archive, delete and association command, plus
  `import`, refuses and sends nothing unless you pass `--yes` (send it) or `--dry-run`
  (preview the request). The refusal exits 2 and, under `--json`/`--agent`, prints a
  JSON envelope with `"confirmation_required": true`. A `--yes` saved in a run profile
  does not count. POSTs that only read (object search, batch read, list search, list
  membership read) run without it.
- **`contacts bulk-update` has its own gate.** Up to 100 rows, a live run needs `--yes`.
  Above 100 rows it prints a digest and requires `--confirm N --digest blast-<hex>`. A
  row counts as updated only when HubSpot confirms it; anything else exits 6 unless you
  pass `--allow-partial-failure`.
- **`--yes` is the approval.** An agent that can run the CLI can also pass `--yes`, so
  keep the human approval step in its policy. `--agent` produces JSON and sets
  `--no-input`; it does not pass `--yes`.
- **Over MCP**, `hubspot_execute` previews every state-changing call and sends it only
  with `confirm` set to boolean `true`. No MCP tool can pass `--yes`, so `import` and a
  live `contacts bulk-update` cannot run over MCP. `HUBSPOT_MCP_READ_ONLY=1` removes the
  write tools from the list.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Local-store-only writes exist and never reach HubSpot.** `teach`, `teach-pattern`,
  `teach-lookup`, `teach-playbook`, `playbook amend` and `learnings confirm|forget|reject|purge`
  write only to the CLI's own SQLite store (the self-learning loop described in AGENTS.md),
  and `export` writes API data to a local file named with `--output` or to stdout. Over MCP
  every file-destination flag (`--output`, `--db`, ...) is refused, so an MCP client can only
  stream `export` into its own result. None of these send a mutation to HubSpot.

## Permission tiers

The CLI refuses every portal write without `--yes` or `--dry-run`, but an agent that
runs the CLI can pass `--yes`, so the real control is still the **agent-level policy
you set** plus the **scope you grant the token**. The recommended agent policy is:
read freely; for any mutation, preview with `--dry-run`, show the exact command and get
human approval before sending it with `--yes`; never run destructive or
credential-touching commands unattended.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Local store only** | Writes to the CLI's own SQLite/state files; nothing reaches HubSpot. | `teach`, `teach-pattern`, `teach-lookup`, `teach-playbook`, `playbook amend`, `learnings confirm`, `learnings forget`, `learnings reject`, `learnings purge`, `export` | Allow; `learnings forget` / `learnings purge` only discard locally taught shortcuts and are undone by teaching again |
| **Write (routine)** | Day-to-day mutations. Each one needs `--yes` (or `--dry-run` to preview); `contacts bulk-update` adds the digest confirm above 100 rows. | `batch post-crm-v3-objects-object-type-archive-archive`, `batch post-crm-v3-objects-object-type-create-create`, `batch post-crm-v3-objects-object-type-update-update`, `contacts bulk-update`, `crm post-v4-associations-from-object-type-to-object-type-batch-archive-archive`, `crm post-v4-associations-from-object-type-to-object-type-batch-associate-default-create-default`, `crm post-v4-associations-from-object-type-to-object-type-batch-create-create`, `crm post-v4-associations-from-object-type-to-object-type-batch-labels-archive-archive-labels`, ... (88 total) | Make the agent pass `--dry-run` to preview, then require human approval of the exact command before it sends with `--yes`. |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `crm delete-v4-objects-object-type-object-id-associations-to-object-type-to-object-id-archive`, `groups delete-crm-v3-properties-object-type-name-archive`, `hubspot-calls-crm delete-v3-objects-calls-call-id-archive`, `hubspot-companies-crm delete-v3-objects-companies-company-id-archive`, `hubspot-contacts-crm delete-v3-objects-contacts-contact-id`, `hubspot-contacts-crm post-v3-objects-contacts-gdpr-delete`, `hubspot-deals-crm delete-v3-objects-0-3-deal-id-archive`, `hubspot-emails-crm delete-v3-objects-emails-email-id-archive`, ... (24 total) | Human-in-the-loop only, explicit `--yes` |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

The examples above are not the full list: every other create, update, upsert, merge, association, list-membership and pipeline command is in the Write tier, and every delete, archive and gdpr-delete command is Destructive; `hubspot-cli agent-context` lists each command's HTTP method. All of them need `--yes`.

## How to lock it down

- **Scope the credential** to only what your workflow needs - this is the strongest
  control. A read/report workflow should use a Private App token with read scopes
  only; the CLI then physically cannot run the Write or Destructive tiers regardless
  of what an agent attempts.
- **Set an agent-level approval policy for mutations.** The CLI refuses a write
  without `--yes`, and the agent can pass it, so require your agent to preview with
  `--dry-run`, surface the exact command and get a human OK before it sends with
  `--yes`.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the HubSpot API, and you can read every line of how it does so. The skill is
read-first, the data stays on your machine, and the write surface is bounded by the
scopes on the token you issue.
