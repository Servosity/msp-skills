# connectsecure skill - governance and safety model

> Unofficial. Community-built skill for the ConnectSecure API. Not affiliated with,
> endorsed by, or sponsored by the vendor.
> This page tells an MSP owner exactly what the connectsecure skill can touch and how to
> scope it, so you can decide what to let an AI agent do.

## What it authenticates as

The skill drives the `connectsecure-cli` binary (and `connectsecure-mcp`) against
your tenant's pod (`CONNECTSECURE_BASE_URL`, `https://pod<N>.myconnectsecure.com`).
It authenticates as the ConnectSecure API user whose key you supply:
`CONNECTSECURE_TENANT`, `CONNECTSECURE_CLIENT_ID` and `CONNECTSECURE_CLIENT_SECRET`,
which the CLI exchanges for a short-lived token at `POST /w/authorize` and sends as
`Authorization: Bearer` plus `X-USER-ID` on every call. If you already hold a token
you can set `CONNECTSECURE_ACCESS_TOKEN` and `CONNECTSECURE_USER_ID` instead.

What touches disk, and where:

- **The client secret** is read from the environment (or from a config file only if
  you put it there). It is never logged and never sent anywhere except your pod.
- **The minted token** is cached in your user cache directory with owner-only
  permissions (0600, in a 0700 directory) and reused for at most 30 minutes.
- **`auth set-token`** saves a token you pipe in to the CLI's credentials file.
- **Secret-bearing ConnectSecure data** - credential records (the domain and SSH
  accounts your scans use), integration credentials and integration rules (whose
  params can hold secrets), scheduled jobs (whose rows carry a password field),
  attack-surface results (whose rows carry creds and hashes fields), and the agent
  uninstall secret - is never written to the
  local SQLite mirror or the HTTP response cache. Custom domains are kept out too, as
  a precaution: creating one uploads the TLS private key, and ConnectSecure does not
  document what the `cert_details` it returns contains. A bare `sync` and
  `workflow archive` skip them, and `sync --resources credentials` refuses before
  any request, stores zero rows and exits non-zero by design.
  The deliberate exception is `export` of those resources (`export credentials`,
  `export integration_credentials`, `export integration_rules`, `export scheduler`,
  `export attack_surface_results`, `export custom_domains`), which prints the plaintext records to stdout or
  writes them to the file you name with `-o`, and `tail` on them, which prints each poll to the terminal.
- **Secrets you send** - create or update credentials, integration credentials,
  integration rules or scheduled jobs with the JSON body on stdin (`--stdin`), not
  `--data-params` / `--data-integration-params` / `--data-password`, so the secret stays out of process arguments
  and shell history. `--dry-run` masks secret-named fields in its preview, an API error
  that echoes a secret you sent is withheld instead of printed, and a
  write is never sent to another origin on a redirect (the CLI fails the write
  instead of reporting a success it did not get).
- **Everything else you read** is cached in your user cache directory: a cached
  response is reused for 5 minutes (pass `--no-cache` for a fresh call) and stays on
  disk until it is overwritten or a later write clears the cache. Whatever you `sync`
  stays in the local mirror until you delete it.

## Default-safe behavior

- **`--dry-run` is opt-in - use it.** Mutating commands send immediately unless you pass `--dry-run` first to preview the request without sending. Make your agent's policy: preview, show the exact command, get approval, then run the write.
- **Read commands are safe to run** (reports, rollups, search); they cannot change
  anything. The exception is the credential reads listed below: they change nothing,
  but they return stored secrets in plaintext to whoever asked.
- **Agent mode is explicit.** `--agent` produces JSON for scripting but does not
  add any write gating - the preview-then-approve policy above still applies. See
  AGENTS.md.
- **The MCP server has a read-only switch.** With `CONNECTSECURE_MCP_READ_ONLY=1`
  (exactly `1`), `connectsecure-mcp` refuses every non-GET call and drops the
  `sync`, `export`, `import` and `tail` tools. It still answers GET calls, including the
  credential reads, so it does not replace scoping the API user.
- **`--deliver` sends output off the machine.** Every CLI command accepts
  `--deliver webhook:<url>`, which posts its output to that URL - including the
  output of a credential read. The MCP server does not expose `--deliver` to tool
  calls; an agent with shell access can still pass it, so keep it out of any
  allow-list.
- **MCP tools never read a local file the client names.** The server runs as
  your account, so the flags that take a path to read (`teach` / `teach-playbook`
  notes and playbook files, `import --input`, the `custom-domains create`
  certificate and key, `report-builder` uploads) are not tool parameters; run
  those from the CLI.

## Permission tiers

The safe default for an autonomous agent is **read plus planned (dry-run) writes**;
require a human for anything below the line.

| Tier | What it does | Examples | Recommended agent policy |
| --- | --- | --- | --- |
| **Read** | Reports, rollups, search. No change. | `exposure`, `portfolio`, `blind-spots`, `suppression-audit`, `ad-hygiene`, `qbr`, `cve-impact`, `sync`, `search`, `analytics`, `export` of non-secret resources, every `list` / `get` except the credential reads below | Allow |
| **Write (routine)** | Day-to-day configuration changes. | `tags create` / `update`, `tag-rules create` / `update`, `companies create` / `update`, `report-builder create-job`, `custom-profile create` / `update`, `event-set create` / `update`, and every other `create` / `update` not named in a tier below (`backup-software`, `edr`, `compliance-assessment`, `custom-ticketing-template`, `report-builder update-cover-page` / `update-standard-report-settings` / `upload-template`) | Preview with `--dry-run`, then an approved write |
| **Credential / security** | Reads or changes stored secrets and tokens. | `credentials list` / `get` / `create` / `update`, `integration-credentials list` / `get` / `create` / `update`, `integration-rules list` / `get` / `create` / `update` (rule params), `attack-surface-results list` / `get` (creds and hashes fields), `agents uninstall-secret`, `scheduler list` / `get` / `create` / `update` (job rows and bodies carry a password field), `custom-domains list` / `get` / `create` (TLS certificate details, contents undocumented; create uploads the private key), `agent-credentials-mapping create` / `update` (which credentials a scan agent uses), `company-mappings create` / `update` (which integration credential a client uses), `export credentials`, `export integration_credentials`, `export integration_rules`, `export scheduler`, `export attack_surface_results`, `export custom_domains`, `tail` on any of these, `session` (mints a token from an encoded client secret; pass `--client-auth-token -` to read it from stdin instead of argv), `auth set-token` | Human-in-the-loop only; never inside a blanket allow-all-reads policy |
| **Endpoint-affecting / posture** | Acts on real client machines or networks, or changes what gets reported. | `patching patch-now` (alias `patching create`; installs application or OS patches on client machines, now or scheduled), `scheduler create` / `update` / `update-schedule` (can schedule patching with a forced reboot across agents, companies or the whole tenant), `companies scan`, `companies external-scan` (launches a scan of a client network or its public addresses), `agents reset` (alias `agents create`; moves agents to another company), `suppress-vulnerability create` / `update` (hides a finding from every report), `discovery-settings create` / `update`, `application-baseline-rules create` / `update`, `pii-scan-settings create` / `update`, `agent-discoverysettings-mapping create` / `update`, `attack-surface-domain create` / `update` (change what gets scanned and reported), `assets create` / `update` (`--data-cred-id` / `--data-snmp-cred-id` pick the scan credentials an asset uses, `--data-discoverysettings-id` its discovery settings, and `--data-is-deprecated` drops it from reporting) | Human-in-the-loop only, explicit confirmation |
| **Destructive** | Irreversible data or config loss. | every `delete` - `companies delete`, `assets delete`, `credentials delete`, `integration-credentials delete`, `discovery-settings delete`, `suppress-vulnerability delete` and 16 more - plus `assets bulk-deprecate`, `scheduler remove-schedule` | Human-in-the-loop only, explicit confirmation |
| **Bulk write** | One API write per line of a file. | `import <resource> --input file.jsonl` | Human-in-the-loop only, explicit confirmation. Never unattended |

Several of those reach outside ConnectSecure itself: `patching patch-now` and patch
schedules change software on real client machines, `companies scan` and `companies external-scan`
send scan traffic at a client's network or public addresses, and `import` can run
any write endpoint in bulk. Treat them like a manual RMM push or a penetration
test: the client's approval and yours, in writing, first.

## How to lock it down

- **Use a dedicated API user.** The client ID and secret are issued per ConnectSecure
  user (Settings > Users > (user) > API Key). Issue them to a user created for this
  skill, so you can see its activity and revoke it without touching anyone's login,
  and give that user the narrowest role your workflow needs. Confirm with
  ConnectSecure how your tenant's roles apply to API calls before relying on a role
  to block credential reads, patching, scans or deletes.
- **Keep autonomous agents to Read + previewed writes.** Have a human approve the
  actual write for Write tier and above - the gate lives in your agent's policy,
  not in the binary's defaults.
- **Run the MCP server with `CONNECTSECURE_MCP_READ_ONLY=1`** for any agent that
  only needs answers.
- **Never let an agent run Credential, Endpoint-affecting, Destructive, or Bulk
  write commands unattended.** Treat them like a production database drop: human,
  reviewed, logged.
- **Rotate the client secret if it is ever exposed** (for example after bridging the
  MCP server to a public endpoint for ChatGPT - see mcp-install.md).

## Why an MSP owner can be comfortable

The full source of the CLI and MCP server is in this repository under
[`cli/`](./cli) (Apache-2.0). You supply the credential, the binary uses it against
your ConnectSecure pod, and you can read every line of how it does so. The skill is
read-first, plan-by-default, and scoped to your own account.
