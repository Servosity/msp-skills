# Avanan CLI

**The first command-line tool for Avanan email security  -  with an offline mirror that answers cross-tenant and what-changed questions the API itself cannot.**

Avanan (Check Point Harmony Email & Collaboration) has integrations for XSOAR, Sentinel, n8n, and MCP, but no CLI and nothing that keeps state. This one covers the whole documented surface, including the sectool exception families the published spec omits, and adds a local SQLite mirror that makes triage, campaign, exceptions audit, msp fleet, and timeline possible. It also implements the documented request signature exactly, including the request-string term the docs leave out.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `avanan-cli` and `avanan-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.ps1 | iex
   ```
3. Verify: `avanan-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=avanan). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/avanan --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/avanan --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `avanan-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the avanan skill from https://github.com/Servosity/msp-skills/tree/main/skills/avanan. The skill defines how its required CLI (`avanan-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=avanan).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `AVANAN_APP_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Interim note:** check any `.mcpb` bundle before you trust it ([#287](https://github.com/Servosity/msp-skills/issues/287)). Its `manifest.json` launches `${__dirname}/bin/avanan-mcp`, while the builder stores the release binaries in `bin/` under their platform-suffixed names - `avanan-mcp-darwin-arm64`, `-darwin-amd64`, `-linux-arm64`, `-linux-amd64`, `-windows-amd64.exe`. Run `unzip -l <file>.mcpb | grep bin/`: if the name the manifest launches is not among them, Claude Desktop has nothing to run - use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_APP_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Avanan uses one of two schemes depending on your host, and the CLI picks automatically. On legacy `*.avanan.net` hosts it performs the signed handshake: a fresh request UUID, your application ID, a GMT timestamp, and an `x-av-sig` computed as sha256(base64(reqId + appId + date + requestString + secret)). The resulting token lasts one hour and is refreshed for you. On Infinity Portal hosts it exchanges your access key for a bearer token instead. Set `AVANAN_APP_ID` and `AVANAN_CLIENT_SECRET`, pick your region, and run `auth login`. Credentials are region-scoped: a US key cannot read EU data, by design.

## Quick Start

```bash
# Confirm which auth scheme, region, and host the CLI resolved before spending a credential
avanan-cli doctor --dry-run

# See which {farm}:{tenant} scopes your app client can reach  -  everything else depends on this
avanan-cli scopes

# Mirror a week of detections, entities, and exceptions locally so the offline commands have data
avanan-cli mirror --since 7d

# The shift-start view: what is new, bucketed by type, severity, and state
avanan-cli triage --since 24h

# Collapse that volume into candidate campaigns before you start clicking
avanan-cli campaign --since 7d

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`triage`**  -  See everything detected in a window you choose, bucketed by threat type, severity, and state, with the sender domains driving the volume.

  _Reach for this at the start of a shift instead of re-querying the same window and eyeballing which detections you already handled._

  ```bash
  avanan-cli triage --since 24h --agent
  ```
- **`campaign`**  -  Group detections into candidate phishing campaigns by sender domain and normalized subject, showing recipient spread and how many are still un-remediated.

  _Reach for this when triage shows volume and you need to know whether forty detections are one campaign or forty problems._

  ```bash
  avanan-cli campaign --since 7d --agent
  ```
- **`timeline`**  -  Reconstruct one message's history from the local mirror: detection, the latest mirrored state, actions this CLI submitted, task outcomes, and restore disposition, in order. Actions taken in the Avanan portal or by another tool are not in the mirror and will not appear.

  _Reach for this when a user disputes a quarantine or a ticket needs evidence of what actually happened to a message._

  ```bash
  avanan-cli timeline f05b74da3ee859eea41aeac40aaad3c2 --agent
  ```

### Collapsing the seven-headed exception system
- **`exceptions find`**  -  Ask whether a domain, sender, URL, or hash is excepted anywhere across all seven engines and nine exception tables at once.

  _Reach for this before adding an allowlist entry, or when a phish got through and you need to know whether one of your own exceptions let it._

  ```bash
  avanan-cli exceptions find partner-domain.com --agent
  ```
- **`exceptions audit`**  -  Flag exceptions that contradict each other across sub-systems, exact duplicates, and entries that have not matched traffic in the mirrored window.

  _Reach for this on a policy review to find the contractor domain nobody removed and the string that is allowlisted in one engine and blocked in another._

  ```bash
  avanan-cli exceptions audit --agent
  ```

### MSP scale
- **`msp fleet`**  -  One ranked table across every tenant joining licensed seats, add-ons, usage, and detection volume.

  _Reach for this to spot tenants over their seat count or with an unusual detection week without walking the tenant list by hand._

  ```bash
  avanan-cli msp fleet --agent
  ```

### Fixing the API's worst footgun
- **`remediate`**  -  Quarantine or restore a batch, wait for the async task to finish, and report the real per-item outcome.

  _Reach for this instead of firing an action and polling by hand, and to turn the single-scope 400 into an error that names your actual scopes._

  ```bash
  avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --wait --dry-run
  ```

## Recipes

### Shift handoff in one command

```bash
avanan-cli triage --since 12h --agent
```

Requires a populated mirror (`mirror --since 7d`). Everything detected in the window, bucketed by type, severity, and state, with the top offending sender domains and a count of anything still unresolved.

### Narrow a large event query to just what an agent needs

```bash
avanan-cli event query --request-data-event-types phishing --agent --select responseData.eventId,responseData.type,responseData.state,responseData.entityInfo.subject,responseData.entityInfo.senderEmail
```

Avanan event payloads nest entity detail several levels deep and run to tens of KB per call; the dotted --select paths keep only the fields worth reading.

### Check an allowlist request before you grant it

```bash
avanan-cli exceptions find vendor-domain.com --agent
```

Requires `mirror --resources exceptions` first. Searches all nine exception tables at once and shows which engine, which list side, and who created each hit  -  replacing nine separate API calls.

### Quarantine a confirmed campaign and prove it landed

```bash
avanan-cli remediate quarantine --entity f05b74da3ee859eea41aeac40aaad3c2 --wait --dry-run
```

Resolves the required single scope, submits the action, polls the task to a terminal state, and prints the per-item outcome. Drop --dry-run to execute.

### Find tenants running over their licensed seats

```bash
avanan-cli msp fleet --agent --select rows.tenant,rows.seats_licensed,rows.seats_used,rows.detections
```

Requires `sync --resources msp` and `mirror --since 7d` first. One local join across mirrored tenants, licenses, and detection volume, ranked by seat overage.

## Usage

Run `avanan-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `AVANAN_CONFIG_DIR`, `AVANAN_DATA_DIR`, `AVANAN_STATE_DIR`, or `AVANAN_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `AVANAN_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export AVANAN_HOME=/srv/avanan
avanan-cli doctor
```

Under `AVANAN_HOME=/srv/avanan`, the four dirs resolve to `/srv/avanan/config`, `/srv/avanan/data`, `/srv/avanan/state`, and `/srv/avanan/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "avanan": {
      "command": "avanan-mcp",
      "env": {
        "AVANAN_HOME": "/srv/avanan"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `AVANAN_DATA_DIR` overrides an explicit `--home` for that kind. Use `AVANAN_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `AVANAN_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `avanan-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### action

Manage action

- **`avanan-cli action post-entity`** - Post Entity Action.
- **`avanan-cli action post-event`** - Post Event Action.

### avanan-search

Manage avanan search

- **`avanan-cli avanan-search get-saas-entity`** - Get Saas Entity.
- **`avanan-cli avanan-search query-saas-entity`** - Query Saas Entity.

### download

Manage download

- **`avanan-cli download <entity_id>`** - Download the raw .eml for a SaaS entity. Set original=true for the unmodified message; the default adds Avanan's visibility headers.

### download-large-email

Manage download large email

- **`avanan-cli download-large-email <entity_id>`** - Return a presigned URL for downloading a message too large to stream inline.

### event

Manage event

- **`avanan-cli event get`** - Get Event.
- **`avanan-cli event query`** - Query Event.

### exceptions

Manage exceptions

- **`avanan-cli exceptions create`** - Create Ap Exception Blacklist.
- **`avanan-cli exceptions create-whitelist`** - Create Ap Exception Whitelist.
- **`avanan-cli exceptions get-ap`** - Get Ap Exception.
- **`avanan-cli exceptions get-ap-exctype`** - Get Ap Exception.
- **`avanan-cli exceptions update`** - Update Ap Exception Blacklist.
- **`avanan-cli exceptions update-whitelist`** - Update Ap Exception Whitelist.

### msp

Manage msp

- **`avanan-cli msp create`** - Create Msp.
- **`avanan-cli msp create-tenants`** - Create Tenant.
- **`avanan-cli msp create-users`** - Create User.
- **`avanan-cli msp delete`** - Delete Msp.
- **`avanan-cli msp delete-tenants`** - Delete Tenant.
- **`avanan-cli msp delete-users`** - Delete User.
- **`avanan-cli msp describe-tenant`** - Describe Tenant.
- **`avanan-cli msp describe-user`** - Describe User.
- **`avanan-cli msp list`** - List Msps.
- **`avanan-cli msp list-addons`** - List Addons.
- **`avanan-cli msp list-daily-usages`** - List Daily Usages.
- **`avanan-cli msp list-licenses`** - List Licenses.
- **`avanan-cli msp list-monthly-usages`** - List Monthly Usages.
- **`avanan-cli msp list-tenants`** - List Tenants.
- **`avanan-cli msp list-users`** - List Users.
- **`avanan-cli msp update`** - Update User.
- **`avanan-cli msp update-or-create-tenant-license`** - Update Or Create Tenant License.

### report

Manage report

- **`avanan-cli report`** - Report that one or more entities were mis-classified (phishing, spam, clean, or marketing email).

### scopes

Manage scopes

- **`avanan-cli scopes`** - Get Scopes.

### sectool-exceptions

Manage sectool exceptions

- **`avanan-cli sectool-exceptions create`** - Create an exception for the named security engine.
- **`avanan-cli sectool-exceptions delete`** - Delete an exception for the named security engine.
- **`avanan-cli sectool-exceptions update`** - Update an existing exception for the named security engine.

### sectools

Manage sectools

- **`avanan-cli sectools create-anomaly-exception`** - Create an Anomaly engine exception rule.
- **`avanan-cli sectools create-ctp-item`** - Add an entry to a Click-Time Protection exception list.
- **`avanan-cli sectools delete-anomaly-exceptions`** - Delete Anomaly engine exception rules by rule ID.
- **`avanan-cli sectools delete-ctp-item`** - Delete one Click-Time Protection exception entry.
- **`avanan-cli sectools delete-ctp-items`** - Delete multiple Click-Time Protection exception entries by ID.
- **`avanan-cli sectools delete-ctp-lists`** - Delete every Click-Time Protection exception list.
- **`avanan-cli sectools get-ctp-item`** - Get one Click-Time Protection exception entry by ID.
- **`avanan-cli sectools get-ctp-list`** - Get one Click-Time Protection exception list by ID.
- **`avanan-cli sectools list-anomaly-exceptions`** - List the Anomaly engine's exception rules.
- **`avanan-cli sectools list-ctp-items`** - List the individual entries across Click-Time Protection exception lists.
- **`avanan-cli sectools list-ctp-lists`** - List the Click-Time Protection exception lists.
- **`avanan-cli sectools update-ctp-item`** - Update one Click-Time Protection exception entry.
- **`avanan-cli sectools update-ctp-items`** - Update Click-Time Protection exception list entries.

### soar

Manage soar

- **`avanan-cli soar get-entity`** - Retrieve the decoded email record for a SaaS entity, including headers and body. **Known limitation:** returns HTTP 404 for every entity on the tenants tested so far, including ones confirmed to exist moments earlier, so treat it as unavailable. `avanan-cli avanan-search get-saas-entity <entity_id> --scopes <scope>` returns the same record and works. The SOAR helpers were reconstructed from the vendor reference guide because the published spec omits them, so the path may simply be wrong, or the feature may be licensed separately.
- **`avanan-cli soar post-notify`** - Send an exposure notification about a specific entity to a list of email addresses.

### task

Manage task

- **`avanan-cli task <task_id>`** - Get Task.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`avanan-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`avanan-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`avanan-cli learnings list`** - Inspect taught rows
- **`avanan-cli learnings forget <query>`** - Undo a teach
- **`avanan-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`avanan-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`avanan-cli teach-pattern`** - Install a query/resource template up front
- **`avanan-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `AVANAN_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `avanan-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
avanan-cli event get <event_id> --scopes <farm>:<tenant> --x-av-req-id 550e8400-e29b-41d4-a716-446655440000

# JSON for scripting and agents
avanan-cli event get <event_id> --scopes <farm>:<tenant> --x-av-req-id 550e8400-e29b-41d4-a716-446655440000 --json
# Filter to specific fields
avanan-cli event get <event_id> --scopes <farm>:<tenant> --x-av-req-id 550e8400-e29b-41d4-a716-446655440000 --json --select actions,additionalData,availableEventActions

# Dry run  -  show the request without sending
avanan-cli event get <event_id> --scopes <farm>:<tenant> --x-av-req-id 550e8400-e29b-41d4-a716-446655440000 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
avanan-cli event get <event_id> --scopes <farm>:<tenant> --x-av-req-id 550e8400-e29b-41d4-a716-446655440000 --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
avanan-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `avanan-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/avanan-cli/config.toml`; `--home`, `AVANAN_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `AVANAN_APP_ID` | auth_flow_input | Yes | Avanan Application ID (the x-av-app-id value) issued by Check Point support. |
| `AVANAN_CLIENT_SECRET` | auth_flow_input | Yes | Set during initial auth setup. |
| `AVANAN_TOKEN` | harvested | No | Populated automatically by auth login. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `avanan-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `avanan-cli doctor` to check credentials
- Verify the environment variable is set: `echo $AVANAN_APP_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **Every request returns 401 even though the credential is correct**  -  The signature covers the request string including the query. Re-run 'avanan-cli auth login', which prints the scheme and host it resolved, and confirm the region matches where your keys were issued.
- **An action returns HTTP 400 with an unhelpful body**  -  Action endpoints accept only one scope. Pass --scope <farm>:<tenant>, or use 'remediate', which resolves the scope for you and lists your scopes when it cannot.
- **Requests worked an hour ago and now return 401**  -  The session token lives one hour. Run 'avanan-cli auth login' to mint a new one; normal commands also refresh it automatically.
- **A tenant's data is missing entirely**  -  Regions are hard-isolated and credentials are per-region. Re-run 'avanan-cli auth login --region <region>' and check the tenant appears in 'avanan-cli scopes'.
- **HTTP 429 during a large mirror**  -  Lower the pace with --rate-limit or narrow the window with 'mirror --since 24h'; the local mirror means you only pay for each window once.
- **triage or campaign reports no events**  -  Events are a POST-query resource that 'sync' cannot walk. Run 'avanan-cli mirror --since 7d' to populate them.
- **'exceptions audit' reports an entry as never matched, but you know it fired**  -  The never-matched check only sees your mirrored window. Widen it with 'avanan-cli mirror --since 30d' and re-run.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**demisto/content CheckPointHEC**](https://github.com/demisto/content/tree/master/Packs/CheckPointHEC)  -  Python
- [**avanan-legacy-mcp**](https://github.com/wyre-technology/avanan-legacy-mcp)  -  TypeScript
- [**avanan-mcp**](https://github.com/wyre-technology/avanan-mcp)  -  TypeScript
- [**Azure-Sentinel Harmony Email connector**](https://github.com/Azure/Azure-Sentinel/tree/master/Solutions/Checkpoint%20Harmony%20Email%20and%20Collaboration)  -  Python
- [**RewstPS Invoke-AvananRestMethod**](https://github.com/gocovi/RewstPS)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)

## Known Gaps

- **Not yet exercised against a live Avanan tenant.** This CLI was verified
  against mock responses, dry-run paths, and a seeded local store, but no
  command has been run with real credentials. The request-signing algorithm is
  pinned by test to the worked example published in the Avanan API Reference
  Guide, and the endpoint set is drawn from that guide plus three shipping
  clients (Cortex XSOAR, Microsoft Sentinel, and two MCP servers)  -  but the
  end-to-end handshake has not been observed against a live tenant. Run
  `avanan-cli doctor` and `avanan-cli auth login` first and report
  anything that misbehaves.
- **`x-av-app-id` is treated as opaque.** The vendor's docs are internally
  inconsistent: the curl sample sends `myapp29` while the signature worked
  example concatenates `US:myapp29`. This CLI signs and sends exactly the value
  you configure, which is the only self-consistent behavior. If you get a 401
  with a credential you believe is correct, try the other form.
- **The offline commands read a local mirror, not the API.** `triage`,
  `campaign`, `timeline`, `exceptions find`, and `exceptions audit` return empty
  results until you run `avanan-cli mirror --since 7d`; `msp fleet` also needs
  `avanan-cli sync --resources msp`. This is by design  -  it is what lets them
  answer cross-tenant and historical questions without spending API quota.
- **`exceptions audit`'s never-matched check is bounded by the mirrored window.**
  An entry listed there is unused *within what you mirrored*, not proven dead.
  Widen with `mirror --since 30d` before acting on it.
- **`timeline` only sees actions this CLI submitted.** The API returns no
  task-to-entity linkage, so the CLI records it at submission time. Quarantines
  performed in the web portal or by another tool will not appear.
- **MCP transport is stdio-only, deliberately.** The generated HTTP transport
  binds all interfaces without authenticating callers; these credentials can
  quarantine mail across every tenant a key reaches, so HTTP is not enabled.
- **System and audit logs are out of scope.** Check Point exposes those through
  the separate Infinity Portal API, not this one.
