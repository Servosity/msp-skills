# ConnectSecure + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectSecure
> API. Not affiliated with, endorsed by, or sponsored by ConnectSecure.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/connectsecure/">
    <img src="../../docs/assets/video/connectsecure/animated-og.gif" alt="ConnectSecure demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/connectsecure/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every ConnectSecure API operation, plus a local store that answers vulnerability x asset x client questions across your whole client base in one command. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ConnectSecure skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `connectsecure-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `connectsecure-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `connectsecure-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `CONNECTSECURE_MCP_HTTP_TOKEN=<token> connectsecure-mcp --transport http` (it binds loopback and requires that bearer token on every call) and expose it behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `connectsecure-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ConnectSecure MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/connectsecure-v0.1.0/connectsecure-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ConnectSecure release on the [releases page](https://github.com/servosity/msp-skills/releases?q=connectsecure).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install connectsecure@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ConnectSecure Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex`. Then authenticate per the README and run `connectsecure-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh)
```

The installer drops both `connectsecure-cli` (the CLI) and `connectsecure-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
connectsecure-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update connectsecure@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/connectsecure --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/connectsecure --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectsecure-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectsecure skill from https://github.com/servosity/msp-skills/tree/main/skills/connectsecure. The skill defines how its required CLI (`connectsecure-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

ConnectSecure hosts each tenant on its own pod, so the CLI needs your pod URL plus the API client credentials from the portal (Settings > Users > (user) > API Key for the client ID and secret; Profile > API Documentation for the pod URL):

```bash
export CONNECTSECURE_BASE_URL=https://pod<N>.myconnectsecure.com
export CONNECTSECURE_TENANT=<the tenant name you type at portal login>
export CONNECTSECURE_CLIENT_ID=<client id>
export CONNECTSECURE_CLIENT_SECRET=<client secret>
connectsecure-cli doctor
```

On first use the CLI exchanges those for a short-lived token (`POST /w/authorize`) and reuses it for at most 30 minutes. If you already hold a token, set `CONNECTSECURE_ACCESS_TOKEN` and `CONNECTSECURE_USER_ID` instead of the tenant, client ID and secret.

`doctor` checks config, paths, and API reachability, and with the tenant, client ID and secret set it mints a token, so a wrong secret shows up as `FAIL API: client init error: ConnectSecure rejected the credentials (HTTP 401)`. It exits 0 even then, so scripts should add `--fail-on error`. A read command such as `connectsecure-cli companies list --limit 1` is the end-to-end check.

### Sync before you ask

`exposure`, `portfolio`, `blind-spots`, `suppression-audit` and `ad-hygiene` read a local mirror. Populate it once:

```bash
connectsecure-cli sync
```

A bare `sync` reads a curated set of 15 resources (companies, assets, agents, vulnerability, remediation, suppression, risk score and Active Directory reports) with GET requests only, and writes them to a SQLite file under your own user account. Credential records, integration secrets and integration-rule params, scheduled jobs (whose rows carry a password field), attack-surface results (whose rows carry creds and hashes fields), custom domains (a precaution: their certificate details are undocumented) and the agent uninstall secret are never written to it; naming one in `--resources` is refused before any request and stores nothing (`sync --resources credentials` on its own exits non-zero). Refresh with `connectsecure-cli sync --full`: a bare re-run only adds and updates rows, so a finding remediated since the last sync stays in the mirror (and in `exposure` and `portfolio`) until a `--full` run prunes it. ConnectSecure's API has no incremental filter, so `--full` reads the same pages a bare run does. The commands warn when the mirror is older than `--max-age` (30 minutes by default).

## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which open vulnerabilities across every client are most likely to be exploited? | `connectsecure-cli exposure --min-epss 0.5 --severity Critical,High` |
| What on the CISA Known Exploited list showed up this week? | `connectsecure-cli exposure --kev --new-since 7d` |
| Which clients carry the most risk, side by side? | `connectsecure-cli portfolio --sort max-epss` |
| A CVE just made the news. Which clients and assets does it hit, and is there a fix? | `connectsecure-cli cve-impact "CVE-2024-3400"` |
| Which assets have scan data we cannot trust? | `connectsecure-cli blind-spots --stale 14d --agent-stale 48h` |
| Which suppressions expired, never expire, or were approved by the person who asked for them? | `connectsecure-cli suppression-audit --within 14d` |
| Which clients have risky Active Directory accounts or weak password policies? | `connectsecure-cli ad-hygiene --stale-logon 90d --min-length 12` |
| What goes in this client's QBR risk slide? | `connectsecure-cli qbr "Acme Dental" --since 90d` |
| Is my auth and connectivity actually working? | `connectsecure-cli doctor` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

A stateless wrapper over the ConnectSecure API answers one endpoint at a time, for one client, right now. The questions an MSP actually asks are cross-client: rank every open finding across all 40 clients at asset level, show which of them a new CVE hits, list every asset whose scan you cannot trust. ConnectSecure's per-client vulnerability report names the client but lists affected assets only by ID, so getting from a finding to the machine, its IP and its scan status is a join you would otherwise do by hand.

This skill syncs a curated set of ConnectSecure reports into a **local SQLite mirror** and does the joining for you. `exposure` joins vulnerability findings to assets and companies, `portfolio` merges assets, findings, suppressions, findings open past 30 days and risk score into one row per client, and `blind-spots` crosses discovery times, scan authentication and agent pings - instant, offline, and the AI sees the answer, not the raw data.

Two commands deliberately stay live: `cve-impact` (the CVE report plus the assets-by-problem report, grouped by client) and `qbr` (one client's risk pack). Both list what they could not fetch or attribute instead of dropping it silently. Like every read, their GET responses are cached on disk for 5 minutes, so a re-run inside that window returns the earlier answer; add `--no-cache` right after a fix or suppression. For scripts: with `--agent`, the analysis commands, `list` / `get`, `sync` and `doctor` wrap their output in `{"meta": ..., "results": ...}`, so parse `.results`; with `--json`, `list` / `get` use the same envelope while `qbr` and `cve-impact` print their report object at the top level. `export` and `tail` stream bare JSON records in either mode.

## The pain this closes

"Vulnerability scans rarely come back clean, so MSPs are often staring at thousands of findings across dozens of customers" ([ChannelPro, "Too many vulnerabilities, not enough time"](https://www.channelpronetwork.com/2026/08/02/how-msps-can-manage-vulnerability-data/), updated 2026). In the same piece ConnectSecure's CEO describes the goal as narrowing 10,000 vulnerabilities to the 100 worth working in the next 30 days, which ConnectSecure approaches with AI-ready reporting in its portal. This skill works on the same problem from the AI agent you already use:

- Ranking findings across every client at asset level means matching each report's asset IDs to real machines. `connectsecure-cli exposure` does the join and sorts by EPSS, then CVSS, then known-exploited status.
- "Which of our clients does this CVE hit?" takes two reports and a manual grouping by client. `connectsecure-cli cve-impact` answers it in one command and names the fixes.
- A clean result on an asset that was not discovered in weeks, was scanned without credentials, or whose agent went quiet is unknown, not clean. `connectsecure-cli blind-spots` lists those assets with a reason code each.
- A suppression that expired but was never lifted, has no end date, or was approved by the person who requested it keeps a real finding out of every report. `connectsecure-cli suppression-audit` flags them, riskiest first.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ConnectSecure MCP server runs locally, but it speaks HTTP natively: start it with `CONNECTSECURE_MCP_HTTP_TOKEN=<token> connectsecure-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp`, refuses to start without that token, and answers 401 to any call that does not send it as a Bearer header) and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ConnectSecure credentials once.

### Is my ConnectSecure data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror and a cache of API responses (each reused for 5 minutes, kept until overwritten) sit in directories under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Where do I find my pod URL and API credentials?

In the ConnectSecure portal: **Settings > Users > (user) > API Key** gives you the client ID and client secret, and **Profile > API Documentation** shows your pod URL (`https://pod<N>.myconnectsecure.com`). The tenant name is the one you type on the portal login screen. Each tenant lives on its own pod, so there is no working default URL - `doctor` tells you when `CONNECTSECURE_BASE_URL` is missing.

### Will this hit my ConnectSecure API rate limits?

Lightly. `sync` reads a curated set of reports once and stores them locally, and `exposure`, `portfolio`, `blind-spots`, `suppression-audit` and `ad-hygiene` then answer from that mirror without calling the API. `qbr` and `cve-impact` call the live API, with GET responses cached on disk for 5 minutes (`--no-cache` forces a fresh call); `cve-impact` reads at most `--max-pages` pages of 100 affected assets. By default the CLI paces itself to the server's rate-limit headers (`--rate-limit`).

### Does it store my scan credentials?

No. Credential records (the domain and SSH accounts your scans use), integration secrets and integration-rule params, scheduled jobs (whose rows carry a password field), attack-surface results (whose rows carry creds and hashes fields), custom domains (a precaution: their certificate details are undocumented) and the agent uninstall secret are never written to the local mirror or the HTTP response cache - not by a bare `sync`, not by `workflow archive`, and not by `sync --resources credentials`, which stores nothing and exits non-zero by design. They are returned only to the command or agent that asked for them, which is why those reads sit in the credential tier below. The deliberate exception is `export` of those resources (`export credentials`, `export integration_credentials`, `export integration_rules`, `export scheduler`, `export attack_surface_results`, `export custom_domains`), which prints exactly what you asked for to stdout, or to the file you name with `-o`, and `tail` on them, which prints each poll to your terminal. To create or update credentials, integration credentials, integration rules or scheduled jobs, pass the JSON body on stdin with `--stdin` (wrapped as `{"data": {...}}`, the shape `--dry-run` shows for the flag form; `--stdin` sends it exactly as given) rather than `--data-params` / `--data-integration-params` / `--data-password`, so the secret stays out of process arguments and shell history; `custom-domains create` takes the private key as a file path (`--ssl-privkey <file>`). The CLI's own API token is cached under your user cache directory (owner-only permissions) for at most 30 minutes.

### Will this replace the ConnectSecure portal?

No. Scanning, remediation, agent deployment and branded client reports stay in the portal. This skill answers the cross-client questions the portal makes you click through - exposure ranking, CVE blast radius, blind spots, suppression hygiene, AD hygiene - from your AI.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `exposure`, `portfolio`, `blind-spots`, `suppression-audit`, `ad-hygiene`, `qbr`, `cve-impact`, `sync`, `search`, every `list` / `get` **except the credential reads below** | Allow |
| Write (routine) | `tags create`, `tag-rules update`, `companies update`, `custom-profile create`, `report-builder create-job`, the `report-builder` uploads (`update-cover-page`, `update-standard-report-settings`, `upload-template`), and every other `create` / `update` not named in a tier below | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `credentials list` / `get` / `create` / `update`, `integration-credentials list` / `get` / `create` / `update` (stored scan and integration secrets, returned in plaintext), `integration-rules list` / `get` / `create` / `update`, `attack-surface-results list` / `get` (creds and hashes fields), `agents uninstall-secret`, `scheduler list` / `get` / `create` / `update` (job rows and bodies carry a password field), `custom-domains list` / `get` / `create` (TLS certificate details; create uploads the private key), `agent-credentials-mapping create` / `update` (which credentials a scan agent uses), `company-mappings create` / `update` (which integration credential a client uses), `export credentials` / `integration_credentials` / `integration_rules` / `scheduler` / `attack_surface_results` / `custom_domains`, `tail` on any of these, `session`, `auth set-token` | Human-in-the-loop only, never in a blanket allow-all-reads policy |
| Endpoint-affecting / posture | `patching patch-now` (alias `patching create`; pushes a patch to real client machines), `scheduler create` / `update` / `update-schedule` (can schedule patching with a forced reboot, tenant-wide), `companies scan`, `companies external-scan` (launches a scan of a client network or its public addresses), `agents reset` (alias `agents create`; moves agents to another company), `suppress-vulnerability create` / `update` (hides a finding from every report), `discovery-settings`, `application-baseline-rules`, `pii-scan-settings`, `agent-discoverysettings-mapping` and `attack-surface-domain` `create` / `update` (change what gets scanned and reported), `assets create` / `update` (pick an asset's scan credentials and discovery settings, or deprecate it out of reporting) | Human-in-the-loop only, explicit confirmation |
| Destructive | every `delete` (`companies`, `assets`, `credentials` and 19 more), `assets bulk-deprecate`, `scheduler remove-schedule`, `import` (one write per line of a file) | Human-in-the-loop only, explicit confirmation |

For agents that use the MCP server, set `CONNECTSECURE_MCP_READ_ONLY=1`: the server then refuses every non-GET call and drops `sync`, `export`, `import` and `tail`. It still answers GET calls, including the credential reads, so it is not a substitute for scoping the API user. Agents with shell access can also add `--deliver webhook:<url>` to any CLI command, which posts its output to that URL; the MCP server does not expose `--deliver` to tool calls. MCP tools also never take a local file path for the CLI to read (`teach` / `teach-playbook` notes and playbook files, `import --input`, the `custom-domains create` certificate and key, `report-builder` uploads); run those from the CLI.

The strongest control is the **scope you grant the ConnectSecure credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ConnectSecure API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-13._
