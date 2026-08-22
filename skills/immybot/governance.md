# immybot skill - governance and safety model

> Unofficial. Community-built skill for the ImmyBot API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the immybot skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `immybot-cli` binary (and `immybot-mcp`), authenticating to
Microsoft Entra ID with the OAuth2 client-credentials grant and calling your own
ImmyBot instance. It needs four values: `IMMYBOT_SUBDOMAIN` (your instance, which
also derives the token scope), `IMMYBOT_TENANT_ID`, `IMMYBOT_CLIENT_ID` and
`IMMYBOT_CLIENT_SECRET`. ImmyBot issues no API key of its own: you register an app
in Entra, create a client secret, then add that Enterprise Application's object ID
as an admin person inside ImmyBot.

Where those values live is your choice, and the CLI respects it:

- **Supplied through the environment** (a secret manager, Keychain, or a
  session-scoped variable): the client ID and client secret are **never copied to
  disk**. `auth login` mints a token without persisting the secret you gave it, and
  `doctor` reports the source as `env:*`.
- **Supplied interactively** via `auth login` or `auth set-token`: the credentials
  and the minted tokens are written to a permission-restricted (`0600`)
  `credentials.toml` in your user *data* directory (`~/.local/share/immybot-cli` on
  Linux/macOS), with `config.toml` under the *config* directory
  (`~/.config/immybot-cli`), because that is the whole point of logging in. Run
  `immybot-cli doctor` to see the resolved paths.

Credentials are never logged. The CLI does journal a local learning trail (command
name, flag *shapes*, exit code - no values and no secrets) under your state
directory; `IMMYBOT_NO_LEARN=1` turns it off. Nothing is sent anywhere except Entra
ID and your ImmyBot instance.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you
  pass `--dry-run` first to preview the request without sending. Make your agent's
  policy: preview, show the exact command, get approval, then run the write.
- **Read commands are always safe to run** (the cross-tenant joins, roll-ups,
  search); they cannot change anything.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not add
  any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.

## Permission tiers

ImmyBot is a software-deployment and desired-state platform: some of its commands
run software and scripts on real client machines. The safe default for an
autonomous agent is **read only**; require a human for anything that reaches an
endpoint, changes desired state, or touches a credential.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Cross-tenant joins, roll-ups, triage, search. No change. | `session-triage`, `drift`, `version-spread`, `fleet-diff`, `assignment-explain`, `computer-dossier`, `deployment-health`, `onboarding-stalled`, `psa-reconcile`, `script-blast-radius`, `search`, and every other `list` / `get` except the credential read named below | Allow |
| **Endpoint and fleet execution** | Runs scripts, software, or maintenance on real client machines, or changes the desired state that causes them to run. Highest impact, hard to undo. | **Arbitrary code:** `scripts create-run-adhoc-metascript` (runs a script body you supply against a named computer), `scripts create-run`, `dynamic-provider-types create-test-environment-by-terminal-id-execute-method-by-method`. **Fleet-wide triggers:** `run-immy-service`, `run-immy-service-new` (both take `--tenants` / `--computers`, `--full-maintenance` and `--reboot-preference`), `maintenance-actions create-latest-action-for-computers`, `maintenance-actions create-latest-action-for-tenants`, `schedules create-bulk-run-now`, `schedules run-now create`. **Session control:** `maintenance-sessions actions`, `maintenance-sessions rerun create`, `maintenance-sessions resume create`, `maintenance-sessions create-rerun-v2`, `maintenance-sessions create-cancel`, `maintenance-sessions create-cancel-all`. **Endpoint state:** `computers registry create-keys`, `computers registry create-values` (writes the registry on a live machine), `computers reinventory create`, `provider-links agents create-install-on-computer-by-computer-id` (installs an agent on a named computer), `installer` (agent rekey), `system create-restart-backend`. **Desired state that deploys:** `target-assignments create`, `target-assignments create-global-create`, `target-assignments create-duplicate`, `target-assignments create-duplicates`, `target-assignments override create`, `target-assignments update-by-id`, `target-assignments update-global-by-id`, `target-assignments update-batch-update`. **Payload publishing:** `software create-global-upload`, `software create-local-upload`, `software create-global-fast-create`, `software create-local-fast-create`, `software create-global-by-identifier-versions`, `software create-local-by-identifier-versions`, `software update-global-by-identifier-versions-by-semantic-version`, `software update-local-by-identifier-versions-by-semantic-version`, `inventory-tasks create-local`, `inventory-tasks create-local-by-id`, `inventory-tasks create-local-by-id-scripts` | Human-in-the-loop, explicit confirmation. Preview with `--dry-run`, then an approved run. |
| **Write (data)** | Creates or updates records in ImmyBot. No endpoint reached. | `tenants create`, `tenants update-activate-by-id` (activates a tenant), `persons create`, `groups create`, `tags create`, `schedules create`, `preferences update-application`, `brandings create`, `provider-links create`, `import` | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Mints, reads, or stores tokens, or changes who can do what. Note this includes a READ: `access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id` returns an `AzureTenantAuthDetails` whose model carries `customAppRegSecret`, so it is a credential read, not a routine one. | `access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id`, `auth login`, `auth set-token`, `oauth get-access-tokens`, `oauth create-access-tokens-by-id-refresh`, `access create-update-azure-tenant-auth-details`, `roles create`, `roles update-by-id`, `user-role-assignments` | Human-in-the-loop only |
| **Destructive** | Irreversible data or config loss. | `computers create-bulk-delete`, `computers ephemeral-agent delete`, `brandings delete-by-id`, `change-requests delete-by-id`, `scripts delete-global-by-id`, `software delete-global-by-identifier`, `dynamic-provider-types delete-global-by-id` | Human-in-the-loop only, explicit confirmation |
| **Local config** | Changes only local CLI state, never your ImmyBot instance. | `profile save` / `delete`, `auth logout`, `sync`, `export`, `learnings`, `teach-pattern`, `teach-lookup`, `teach-playbook`, `playbook amend`, `recall`, `feedback` | Allow (no server effect) |

## How to lock it down

- **Scope the Entra app and the ImmyBot admin person** to only what your workflow
  needs. A read/report workflow does not need a credential that can run maintenance
  on endpoints or delete computers.
- **Keep autonomous agents to Read.** Endpoint and fleet execution is the tier that
  separates this connector from a reporting tool: one `create-latest-action-for-tenants`
  reaches every machine in a tenant. Have a human approve those explicitly.
- **Never let an agent run Credential, Destructive, or Endpoint-execution tier
  commands unattended.** Treat them like a production database drop: human,
  reviewed, logged.
- **Prefer environment-supplied credentials** if you do not want a long-lived secret
  on disk; the CLI will not persist them.
- **Rotate the client secret if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
the ImmyBot API, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own instance.
