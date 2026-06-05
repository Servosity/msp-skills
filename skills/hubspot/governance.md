# hubspot skill - governance and safety model

> Unofficial. Community-built skill for the HubSpot API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the hubspot skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `hubspot-cli` binary (and `hubspot-mcp`),
authenticating with `HUBSPOT_ACCESS_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the HubSpot API.

## How writes actually behave (read this before trusting an agent)

- **Writes are not gated by default.** `--dry-run` is an opt-in global flag -
  its help text is *"Show request without sending"* and its default is off. A raw
  create/update/delete/archive command (for example `hubspot-contacts-crm
  post-v3-objects-contacts`) **sends the mutation immediately** on first run with no
  plan and no confirmation. Make your agent pass `--dry-run` first to preview any
  mutation without sending it.
- **There is exactly one built-in mutation gate, and it is conditional.**
  `contacts bulk-update` prints a digest and requires `--confirm N --digest blast-<hex>`
  - but **only for batches above 100 rows**. Its own help: *"Smaller batches are
  dispatched in one pass with a one-line warning that the digest gate was bypassed."*
  Do not rely on this gate for the common small-batch case.
- **`--confirm` exists only on `contacts bulk-update`.** No raw `batch …` or
  `*-crm post-…` write command has a `--confirm` flag. The only confirmation-related
  global is `--yes`, which *skips* prompts for agents - it does not add a gate.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode does not add safety.** `--agent` produces JSON and sets
  `--no-input --yes` for non-interactive use; it relaxes prompts rather than adding a
  confirm step. See AGENTS.md.

## Permission tiers

Because the CLI does not gate writes for you, the real control is the **agent-level
policy you set** plus the **scope you grant the token**. The recommended agent policy
is: read freely; for any mutation, show the exact command and get human approval
before sending; never run destructive or credential-touching commands unattended.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. Raw writes send immediately; `contacts bulk-update` gates only above 100 rows. | `batch post-crm-v3-objects-object-type-archive-archive`, `batch post-crm-v3-objects-object-type-create-create`, `batch post-crm-v3-objects-object-type-update-update`, `contacts bulk-update`, `crm post-v4-associations-from-object-type-to-object-type-batch-archive-archive`, `crm post-v4-associations-from-object-type-to-object-type-batch-associate-default-create-default`, `crm post-v4-associations-from-object-type-to-object-type-batch-create-create`, `crm post-v4-associations-from-object-type-to-object-type-batch-labels-archive-archive-labels`, ... (88 total) | Make the agent pass `--dry-run` to preview, then require human approval of the exact command before sending. Do not assume a built-in gate. |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `crm delete-v4-objects-object-type-object-id-associations-to-object-type-to-object-id-archive`, `groups delete-crm-v3-properties-object-type-name-archive`, `hubspot-calls-crm delete-v3-objects-calls-call-id-archive`, `hubspot-companies-crm delete-v3-objects-companies-company-id-archive`, `hubspot-contacts-crm delete-v3-objects-contacts-contact-id`, `hubspot-contacts-crm post-v3-objects-contacts-gdpr-delete`, `hubspot-deals-crm delete-v3-objects-0-3-deal-id-archive`, `hubspot-emails-crm delete-v3-objects-emails-email-id-archive`, ... (24 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs - this is the strongest
  control. A read/report workflow should use a Private App token with read scopes
  only; the CLI then physically cannot run the Write or Destructive tiers regardless
  of what an agent attempts.
- **Set an agent-level approval policy for mutations.** Since the CLI does not gate
  writes itself, require your agent to surface the exact command and get a human OK
  before any create/update/delete - and to preview with `--dry-run` first.
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
