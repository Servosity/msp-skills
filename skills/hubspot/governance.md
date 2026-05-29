# hubspot skill - governance and safety model

> Unofficial. Community-built skill for the HubSpot API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the hubspot skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `hubspot-cli` binary (and `hubspot-mcp`),
authenticating with `HUBSPOT_ACCESS_TOKEN`. Credentials are read from the environment only -
never written to disk, never logged, never sent anywhere except the HubSpot API.

## Default-safe behavior

- **Mutating commands plan by default.** They run `--dry-run` until you drop it and pass `--confirm`.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  relax the confirm-before-mutate rule. See AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | the cross-entity views and any non-mutating command | Allow |
| **Write (routine)** | Day-to-day mutations. | `batch post-crm-v3-objects-object-type-archive-archive`, `batch post-crm-v3-objects-object-type-create-create`, `batch post-crm-v3-objects-object-type-update-update`, `contacts bulk update`, `crm post-v4-associations-from-object-type-to-object-type-batch-archive-archive`, `crm post-v4-associations-from-object-type-to-object-type-batch-associate-default-create-default`, `crm post-v4-associations-from-object-type-to-object-type-batch-create-create`, `crm post-v4-associations-from-object-type-to-object-type-batch-labels-archive-archive-labels`, ... (88 total) | Allow with `--confirm`; log the plan first |
| **Credential / security** | Touches tokens, keys, MFA. | (none detected) | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `crm delete-v4-objects-object-type-object-id-associations-to-object-type-to-object-id-archive`, `groups delete-crm-v3-properties-object-type-name-archive`, `hubspot-calls-crm delete-v3-objects-calls-call-id-archive`, `hubspot-companies-crm delete-v3-objects-companies-company-id-archive`, `hubspot-contacts-crm delete-v3-objects-contacts-contact-id`, `hubspot-contacts-crm post-v3-objects-contacts-gdpr-delete`, `hubspot-deals-crm delete-v3-objects-0-3-deal-id-archive`, `hubspot-emails-crm delete-v3-objects-emails-email-id-archive`, ... (24 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | Back-office administration. | (none detected) | Operator-only, not for agents |

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive or Credential tiers.
- **Keep autonomous agents to Read + planned writes.** Have a human run the
  `--confirm` step for Write tier and above.
- **Never let an agent run Credential, Destructive, or Admin tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the HubSpot API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
