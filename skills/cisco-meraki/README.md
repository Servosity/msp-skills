# Cisco Meraki + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Cisco Meraki
> API. Not affiliated with, endorsed by, or sponsored by Cisco Systems, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/cisco-meraki/">
    <img src="../../docs/assets/video/cisco-meraki/animated-og.gif" alt="Cisco Meraki demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/cisco-meraki/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Meraki Dashboard operation, plus cross-org fleet views that read every page, and a local mirror built for MSPs running many client orgs. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Cisco Meraki skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `cisco-meraki-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `cisco-meraki-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `cisco-meraki-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, set `CISCO_MERAKI_MCP_HTTP_TOKEN`, run `cisco-meraki-mcp --transport http`, and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `cisco-meraki-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Cisco Meraki MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/cisco-meraki-v0.1.1/cisco-meraki-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Cisco Meraki release on the [releases page](https://github.com/servosity/msp-skills/releases?q=cisco-meraki).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install cisco-meraki@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Cisco Meraki Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex`. Then authenticate per the README and run `cisco-meraki-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)
```

The installer drops both `cisco-meraki-cli` (the CLI) and `cisco-meraki-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
cisco-meraki-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update cisco-meraki@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/cisco-meraki --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/cisco-meraki --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cisco-meraki-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cisco-meraki skill from https://github.com/servosity/msp-skills/tree/main/skills/cisco-meraki. The skill defines how its required CLI (`cisco-meraki-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Generate a Dashboard API key (Meraki Dashboard > My profile > API access) and set it:

```bash
export MERAKI_DASHBOARD_API_KEY=<your-api-key>
cisco-meraki-cli doctor
cisco-meraki-cli fleet health --limit 5
```

The key reaches every organization the admin who generated it can see, with that admin's permissions - generate it from a read-only admin if you only want reporting. Each org needs Dashboard API access turned on (Organization > Settings). `doctor` checks config and reachability and test-calls the key; read its `Credentials` line, because it exits 0 even when the key is rejected. On a regional dashboard, also set `CISCO_MERAKI_BASE_URL` (for example `https://api.meraki.ca/api/v1` for Canada).


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client org is in the worst shape right now? | `cisco-meraki-cli fleet health --agent` |
| What is offline or alerting across every org, and for how long? | `cisco-meraki-cli fleet down --agent` |
| Which MX WAN uplinks have failed? | `cisco-meraki-cli fleet down --kind uplink --json` |
| Which licenses, subscriptions, and end-of-support dates are due in 90 days? | `cisco-meraki-cli fleet renewals --within 90d --agent` |
| Who changed what across all orgs this week? | `cisco-meraki-cli fleet changes --since 7d --group-by admin --agent` |
| Did a change land right before this device went offline? | `cisco-meraki-cli fleet blame "Q2QN-9J8L-SLPD" --window 2h --agent` |
| Which org and network owns this MAC, IP, or serial? | `cisco-meraki-cli fleet find "00:18:0a:12:34:56" --agent` |
| Which admins have two-factor off or stale access, and where does a departed tech still have access? | `cisco-meraki-cli fleet admins --stale 90d --flagged --agent` |

Beyond the fleet views, all 1,035 Dashboard API operations are typed commands under `organizations`, `networks`, `devices`, and `administered`; `cisco-meraki-cli which "offline devices"` finds the right one.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Meraki integrations and MCP servers proxy one question into one live API call against one organization, and often read only the first page of a list. That's fine for one record. It dies at MSP scale, when you're asking "what is down for all 30 clients, and since when?"

This skill is built for that question. The `fleet` commands run across **every org the key can see** under one rate limiter, follow every `Link`-header page, and never count an org that fails to answer as zero devices: `fleet health` keeps it as an unreachable row and the other fleet commands name it on stderr. Compound views do the joins a stateless wrapper can't: `fleet down` joins device status to org and network names and computes downtime, `fleet blame` lines up each outage with the config changes made just before it, and `fleet renewals` merges three licensing models with hardware end-of-support dates into one date-sorted list. The fleet views query the Dashboard live; `sync` also keeps a **local SQLite mirror** of the org-level data so `search "Acme" --data-source local`, `fleet find`, and `fleet changes --data-source local` can answer offline. Single-endpoint reads through the MCP server say when Meraki has more pages (`has_more`, plus the cursor to resume, or how to re-request a page too large for one result).

## The pain this closes

Meraki's MSP Portal (now the Global Overview page), as [Meraki documents it](https://documentation.meraki.com/Platform_Management/Managed_Service_Provider_Resources/Monitoring_and_Managing_Multiple_Organizations), lists each client org with device counts, license status, and one co-term expiration date. It documents no cross-org list of which devices are down, no cross-org change log, and no admin management across orgs - so "what is broken for every client right now?" still means clicking into each org. Scripting it runs into the API's shape: paginated lists come back as bare arrays paged only by a `Link` header, so a one-call script silently stops at page one (the Meraki Community Developers & APIs board has threads like "Using Powershell how to paginate thru pages" on exactly this), all under a ten-calls-per-second, per-org rate limit.

- `cisco-meraki-cli fleet down --agent` - the morning triage list: every offline or alerting device and failed uplink, with network names and downtime.
- `cisco-meraki-cli fleet renewals --within 90d --agent` - the renewal pipeline across co-term, per-device, and subscription orgs plus end-of-support hardware.
- `cisco-meraki-cli fleet blame "Q2QN-9J8L-SLPD" --window 2h --agent` - "did a tech break it?" answered with the changes made just before each outage.
- `cisco-meraki-cli fleet admins --email "departed.tech@msp.example" --json` - every org that still grants a departed tech access.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Cisco Meraki MCP server runs locally, but it speaks HTTP natively: set `CISCO_MERAKI_MCP_HTTP_TOKEN` (callers must send it as a bearer token), start it with `cisco-meraki-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default), and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Cisco Meraki credentials once.

### Is my Cisco Meraki data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What can the API key reach?

A Meraki API key carries the Dashboard permissions of the admin account that generated it, across every organization that admin can see. Generate it from an admin with read-only access if you only want reporting. Each org must also have Dashboard API access enabled (Organization > Settings); an org without it shows up in `fleet health` as `reachable: false` rather than disappearing.

### Will this hit my Meraki API rate limit?

Meraki allows about ten calls per second per organization. The CLI paces itself at 10 requests per second by default (change it with `--rate-limit`) and, when Meraki answers HTTP 429, waits for the `Retry-After` header before trying again. Fleet commands across many orgs take longer: each run has one overall `--timeout` (default 1m), so raise it (for example `--timeout 10m`) for a large fleet. An org that runs out of time is named on stderr as failed, not silently dropped.

### Does it work with the Canada, China, India, or FedRAMP dashboards?

Yes. Set `CISCO_MERAKI_BASE_URL` to your region's API host: `https://api.meraki.ca/api/v1` (Canada), `https://api.meraki.cn/api/v1` (China), `https://api.meraki.in/api/v1` (India), or `https://api.gov-meraki.com/api/v1` (FedRAMP). Leave it unset for the global dashboard.

### Can an agent change my clients' networks by accident?

Writes send immediately when called, with no confirmation prompt, so keep autonomous agents to reads and `--dry-run` previews. The bulk tool, `fleet push`, previews a per-network diff by default and sends nothing unless you add `--apply`. Set `CISCO_MERAKI_MCP_READ_ONLY=1` on the MCP server and it refuses writes outright. The [safety model](#safety-model) below says which commands to keep behind a human.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `fleet down`, `fleet renewals`, `fleet changes`, `fleet find`, `fleet fanout` (except at a secret-bearing path, including `configurationChanges` and `apiRequests`), `fleet admins`, `fleet flaps`, `fleet blame`, `fleet push` without `--apply` (current credential fields print as `[redacted]`), every GET except the secret-bearing ones | Allow |
| Credential (incl. secret-returning reads) | `auth set-token`, `administered generate-identities-me-api-keys`, admin and SAML writes, `organizations login-security update-organization` (IP ranges and two-factor: can lock every admin and API key out), `organizations update` (`api.enabled=false` cuts every API key off), and reads that return live secrets: `networks wireless get-network-ssids` (Wi-Fi PSK), `networks wireless get-network-ssid-identity-psks`, `networks snmp get-network`, `organizations snmp get-organization`, `organizations appliance get-organization-vpn-third-party-vpnpeers`, `organizations sm get-organization-vpp-accounts` (Apple VPP token), `organizations configuration-changes get-organization` (the raw change log carries API request bodies; `fleet changes` is the redacted view), `organizations action-batches get-organization-action-batch` (returns each pushed action's body), `networks webhooks get-network-payload-templates` (header templates can hold an `Authorization` value), `networks webhooks get-network-http-servers` and `organizations webhooks get-organization-logs`, alert profile reads such as `organizations alerts get-organization-profiles` (an incoming-webhook URL, or an HTTP server ID that encodes it, is a credential), `organizations api-requests get-organization` (request paths can carry those IDs), license-key reads such as `organizations licenses get-organization`, and the commands that return a time-limited link (`organizations devices generate-organization-packet-capture-capture-download-url`, `devices camera generate-device-snapshot`, `devices camera clip-device`, `networks floor-plans get-network`, `organizations branding-policies get-organization`, and the floor-plan, branding-policy and custom-analytics-artifact writes that echo one) | Human-in-the-loop only - the CLI does not redact these reads; [governance.md](./governance.md) has the full list |
| Write (routine) | `networks update`, `networks wireless update-network-ssid`, `networks appliance update-network-vlan`, `devices update` (a write that removes anything, replaces a membership list, changes who can sign in or call the API, or affects power, firmware or connectivity is never routine, and neither is any POST ending in an action such as `/bind`, `/move` or `/remove` - see governance.md) | Preview with `--dry-run`, then a reviewed write |
| Bulk and device-disrupting | `fleet push --apply`, `organizations action-batches create-organization-action-batch`, `networks devices vmx-network-claim`, `devices reboot device`, `devices switch cycle-device-ports`, `networks firmware-upgrades update-network`, `networks firmware-upgrades create-network-rollback`, the staged firmware event writes (`networks firmware-upgrades create-network-staged-event` and friends) `devices sensor create-device-command` (MT40 power cycle), and `networks switch create-network-stack` / `add-network-stack` (forming a stack reboots the switches) | Human-in-the-loop only |
| Destructive | every delete, `networks devices remove-network`, `organizations inventory release-from-organization`, `networks sm wipe-network-devices`, `networks switch update-network-stack` (a `members` list replaces the whole stack), `organizations wireless create-organization-zigbee-disenrollment`, every `.../bulkDelete` and `.../batchDelete` POST | Human-in-the-loop only, explicit confirmation |

The strongest control is the **Dashboard admin behind the API key** - the CLI can only do what that admin is permitted to do, so generate the key from a read-only admin for reporting. For an MCP agent that should never write, also set `CISCO_MERAKI_MCP_READ_ONLY=1`: the server then refuses every non-GET call and `fleet push --apply`. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Cisco Meraki API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-13._
