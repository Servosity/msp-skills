# jamf-pro skill - governance and safety model

> Unofficial. Community-built skill for the Jamf Pro API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the jamf-pro skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `jamf-pro-cli` binary (and `jamf-pro-mcp`),
authenticating with a Jamf Pro API client: `JAMF_PRO_SERVER`, `JAMF_PRO_CLIENT_ID` and
`JAMF_PRO_CLIENT_SECRET` for one server, or one registered instance per client with its
secret as a `name=secret` entry in `JAMF_PRO_FLEET_SECRETS`. A secret in the environment is never written
to disk. Minted tokens, and a secret you pass to `auth login --client-secret`, are saved in
one credentials file readable only by your user (mode 0600) and bound to that server.
Credentials go only to that Jamf Pro server, and any output, file or request that repeats
one, in any encoding, is withheld.

## Default-safe behavior

- **`--dry-run` is opt-in on the CLI - use it.** CLI writes send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (reports, rollups, search); they cannot
  change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change to Jamf Pro. | `fleet find`, `fleet posture`, `fleet patch`, `fleet app`, `fleet policy-hygiene`, `fleet health`, `fleet baseline`, `fleet exec`, `computer-inventory list`, `scripts get`, `policies list`, `sync`, `search` | Allow |
| **Write (routine)** | Day-to-day changes. | `buildings create`, `categories update`, `departments update`, `computer-groups create-static-groups`, `packages update`, `computer-extension-attributes update`, every other POST, PUT or PATCH not named below | Preview with `--dry-run`, then a reviewed write |
| **Credential and identity** | API clients and roles, accounts, LAPS settings and history, tokens, and any request that sends a credential. Treat any command that can emit a stored secret field as this tier. | `auth login`, `auth set-token`, every `api-integrations` and `api-roles` command, `accounts`, `account-groups`, every `local-admin-password` command, `volume-purchasing-locations create` and `update`, and any prestage write that sets an admin password | Human-in-the-loop only |
| **Acts on customer devices** | Runs code or changes state on a customer's Mac, iPad or iPhone, or takes back its licenses. | `computer-inventory erase`, `computer-inventory remove-mdm-profile`, `mobile-devices erase`, `mobile-devices unmanage`, `mdm send-command`, `mdm deploy-package`, `mdm blank-push`, `mdm renew-profile`, `managed-software-updates create-plans`, `patch-policy-logs retry`, `policies create` and `update`, `scripts create` and `update`, `device-enrollments disown`, `volume-purchasing-locations revoke-licenses` and `reclaim` | Human-in-the-loop only, one instance at a time |
| **Destructive / config** | Irreversible data or config loss. | Every `delete` and `delete-multiple` command, for example `scripts delete`, `policies delete`, `computer-inventory delete`, `api-roles delete`, `device-enrollments delete` | Human-in-the-loop only, explicit confirmation |

The `fleet` commands are read-only across every instance. A write runs against one instance
at a time, chosen with `--instance`. Endpoints that return LAPS passwords, FileVault keys,
recovery-lock passwords and lock PINs are not included, and `fleet exec` refuses them by path.
In the MCP server every write returns a preview and acts only with `confirm: true`; set
`JAMF_PRO_MCP_READ_ONLY=1` to expose reads only.

## How to lock it down

- **Scope the credential** to only what your workflow needs. A read/report workflow
  does not need a credential that can run the Destructive, Device or Credential tiers. In
  Jamf Pro, give each API role only the privileges your reports read.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Never let an agent run Credential, Device or Destructive tier commands
  unattended.** Treat them like a production database drop: human, reviewed, logged.
- **Rotate the credential if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the Jamf Pro API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
