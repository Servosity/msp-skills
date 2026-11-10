# Cisco Umbrella + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Cisco Umbrella
> API. Not affiliated with, endorsed by, or sponsored by Cisco Systems, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/cisco-umbrella/">
    <img src="../../docs/assets/video/cisco-umbrella/animated-og.gif" alt="Cisco Umbrella demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/cisco-umbrella/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Cisco Umbrella v2 API call, plus MSP child-org switching, domain verdicts, list diffs, and fleet health. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Cisco Umbrella skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `cisco-umbrella-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `cisco-umbrella-mcp` over HTTPS with `CISCO_UMBRELLA_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `cisco-umbrella-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `cisco-umbrella-mcp --transport http` with `CISCO_UMBRELLA_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `cisco-umbrella-mcp` over HTTPS with `CISCO_UMBRELLA_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Cisco Umbrella MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/cisco-umbrella-v0.1.3/cisco-umbrella-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Cisco Umbrella release on the [releases page](https://github.com/servosity/msp-skills/releases?q=cisco-umbrella).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install cisco-umbrella@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Cisco Umbrella Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex`. Then authenticate per the README and run `cisco-umbrella-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)
```

The installer drops both `cisco-umbrella-cli` (the CLI) and `cisco-umbrella-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
cisco-umbrella-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update cisco-umbrella@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/cisco-umbrella --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/cisco-umbrella --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cisco-umbrella-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cisco-umbrella skill from https://github.com/servosity/msp-skills/tree/main/skills/cisco-umbrella. The skill defines how its required CLI (`cisco-umbrella-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API key in the Umbrella dashboard under **Admin > API Keys** (in an MSP or Multi-org console, create it in the parent console), give it the scopes you need, then set the key and secret:

**macOS / Linux:**

```bash
export CISCO_UMBRELLA_API_KEY=<api-key>
export CISCO_UMBRELLA_API_SECRET=<api-secret>
cisco-umbrella-cli doctor
```

**Windows (PowerShell):**

```powershell
$env:CISCO_UMBRELLA_API_KEY = "<api-key>"
$env:CISCO_UMBRELLA_API_SECRET = "<api-secret>"
cisco-umbrella-cli doctor
```

To work inside one customer from a parent console key, add `--org <child org ID>` to any command or set `CISCO_UMBRELLA_ORG_ID`; `cisco-umbrella-cli managed-customers list` shows the IDs.

`doctor` checks config, paths and API reachability, then proves the credential with one read of `/deployments/v2/networks`. A key without that scope reads as scope-limited, which still means the key was accepted.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customers have roaming computers off, uninstalled, stale or without web protection? | `cisco-umbrella-cli fleet health --stale-days 7 --agent` |
| What deployment problems does this one customer have (roaming computers, virtual appliances, tunnels, networks)? | `cisco-umbrella-cli hygiene --org 8675309 --agent` |
| Is this domain blocked, what does Umbrella say about it, and who resolved it? | `cisco-umbrella-cli verdict login.example.com --agent` |
| Which allow-list entries are now malicious, conflict with a block list, or were added this week? | `cisco-umbrella-cli allowlist audit --since 7d --agent` |
| What would change to make a destination list match my blocklist file? | `cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent` |
| How did this customer do over the last 30 days, against the 30 before, for the QBR? | `cisco-umbrella-cli report-card --org 8675309 --from -30days --agent` |
| Which roaming computers match a name, across everything synced? | `cisco-umbrella-cli search "FRONTDESK" --data-source local --agent` |

`hygiene` and `search` read the local store: run `cisco-umbrella-cli sync` first (add `--org <child id>` to sync one customer into its own store).

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Cisco Umbrella integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 40 customers have roaming computers that stopped checking in?" and the answer means switching into 40 child orgs.

This skill syncs Cisco Umbrella into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet health` (a child-org token per customer, then roaming computers, virtual appliances and tunnels rolled into one row each), `verdict` (Investigate, every destination list including parent-domain entries, and the activity report) and `allowlist audit` (allow lists checked against Investigate and against your block lists) join across customers, lists and reports - work a stateless API wrapper can't do.

## The pain this closes

In an Umbrella MSP or Multi-org console, deployment status lives inside each child org. Cisco's own description is that the console lists every child org so it is "easy to switch in and out between customers" ([Cisco partner blog](https://blogs.cisco.com/partner/cisco-umbrella-for-managed-services), December 2022). Switching in is easy; doing it for every customer to find roaming computers that are off or not checking in, a virtual appliance that is not healthy or a tunnel that is down, is a morning of clicking. Triage has the same shape: a domain's category and risk score live in Investigate, its list membership in every destination list, and who resolved it in an activity report.

- `cisco-umbrella-cli fleet health` - one row per managed customer, from a parent console key.
- `cisco-umbrella-cli verdict <domain>` - the whole answer for a domain in one call.
- `cisco-umbrella-cli allowlist audit` - allow-list entries that have turned risky or conflict with a block list.
- `cisco-umbrella-cli destination-lists diff <id> --file <blocklist>` - the plan to make a list match your file; `--apply` makes it so.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Cisco Umbrella MCP server runs locally over stdio by default. Its HTTP mode (`cisco-umbrella-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `CISCO_UMBRELLA_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Cisco Umbrella credentials once.

### Is my Cisco Umbrella data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config, and are never bundled into this repo or transmitted anywhere by MSP Skills. The first API call with them saves the key, the secret and the bearer token to `credentials.toml` in the data directory (`doctor` prints it): owner-only, not encrypted. Later runs use them after you unset the variables, until `auth logout` deletes the file; while the variables stay set, the next API call saves them again. A key named on the run itself (`CISCO_UMBRELLA_API_KEY`, or the config file the run loads) wins over a different key, bearer or `auth set-token` token saved by an earlier run, so naming another customer's key reaches that customer.

### Does it work with an MSP or Multi-org console?

Yes. Create the API key in the parent console. Every command takes `--org <child org ID>` (or `CISCO_UMBRELLA_ORG_ID`) and mints a token for that customer with the `X-Umbrella-OrgId` header; `fleet health` walks every managed customer for you. The child token stays in memory, and each customer's synced data goes to its own local store.

### Which API key scopes does it need?

Only the scopes for what you run: Reports for `activity`, `report-card` and the resolver lookup in `verdict`; Deployments for networks, roaming computers and `fleet health`, which also needs the Admin customers scope; Policies for destination lists; Investigate for categorization and risk scores; Admin for keys and users. A key missing a scope gets a 403. Read-only scopes cannot change anything.

### Will this hit the Umbrella API rate limits?

It is built to stay inside them. Token requests are limited to 20 a minute, so `fleet health` starts customers 3 seconds apart; narrow a large fleet with `--customers` or `--max-customers`. Sync once, and `search`, `hygiene` and `analytics` answer from the local store without new API calls.

### Why can't it create or refresh an API key?

Umbrella returns a newly generated secret only once and cannot show it again, and this program never prints a credential it holds or receives, so the secret never lands in an agent transcript or a log. Together that would destroy the only copy, and a refresh would also break whatever uses the key now. So five actions are refused here, before anything is sent, over the CLI and MCP alike: `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no secret of your own, and `tunnels update-credentials` with auto-rotate or with no secret of your own. Do them in the Umbrella dashboard, where the secret is shown. `--dry-run` still previews them. Reads stay: `api-keys list` and `api-keys get` print the record with every credential-named field (`clientSecret`, `clientId` and the like) replaced by `[redacted]`. Any other output that repeats a credential the program holds, plain or encoded, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`, where `<sink>` names where it was going.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `hygiene`, `verdict`, `allowlist audit`, `report-card`, `destination-lists diff` (without `--apply`), `activity`, `reports`, every `list` and `get`, `sync`, `search` | Allow |
| Write (routine) | `destination-lists add-destinations`/`create`/`update`, `destination-lists diff --apply`, `networks`/`sites`/`internal-networks`/`internal-domains`/`network-devices` `create`/`update`, `tags create`/`add-to-devices`, `policies update-policy-identities`, `tunnels update`, `import` | Preview with `--dry-run`, then a reviewed write |
| Credential / secret-bearing | `api-keys update`, `tunnels add` and `tunnels update-credentials` with a secret you chose, `password-resets`, `users create`, `auth login`, `auth set-token` | Human-in-the-loop only |
| Refused here | `api-keys create`, `api-keys refresh`, `s3-bucket-keys`, `tunnels add` with no secret of your own, `tunnels update-credentials` with auto-rotate or no secret of your own | Do it in the Umbrella dashboard: the new secret is shown only once and this program withholds it |
| Destructive | every `delete`, `destination-lists remove-destinations`, `tags remove-from-devices`, `swg-device-settings remove`, `local-data purge` | Human-in-the-loop only, explicit confirmation |

Over MCP, every write to Umbrella returns a preview until the call carries `confirm: true`, a preview changes nothing on this machine, local learn-store tools (`teach`, `learnings_*`, `playbook_*`) run with no preview and change only this machine, and `CISCO_UMBRELLA_MCP_READ_ONLY=1` removes writes entirely.

The strongest control is the **scope you grant the Cisco Umbrella credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Cisco Umbrella API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
