# Microsoft Exchange Online + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Microsoft Exchange Online
> API. Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/exchange-online/">
    <img src="../../docs/assets/video/exchange-online/animated-og.gif" alt="Exchange Online demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/exchange-online/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Exchange Online admin for MSPs over REST: forwarding audits, EWS posture, offboarding and quarantine across client tenants, no PowerShell session. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Microsoft Exchange Online skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `exchange-online-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `exchange-online-mcp` over HTTPS with `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `exchange-online-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `exchange-online-mcp --transport http` with `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `exchange-online-mcp` over HTTPS with `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Microsoft Exchange Online MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/exchange-online-v0.1.0/exchange-online-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Microsoft Exchange Online release on the [releases page](https://github.com/servosity/msp-skills/releases?q=exchange-online).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install exchange-online@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Microsoft Exchange Online Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex`. Then authenticate per the README and run `exchange-online-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh)
```

The installer drops both `exchange-online-cli` (the CLI) and `exchange-online-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
exchange-online-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update exchange-online@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/exchange-online --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/exchange-online --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `exchange-online-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the exchange-online skill from https://github.com/servosity/msp-skills/tree/main/skills/exchange-online. The skill defines how its required CLI (`exchange-online-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set a bearer token for `https://outlook.office365.com` issued for the customer tenant you are working in (GDAP partners mint one per tenant; tokens last about an hour). App-only tokens need Exchange.ManageAsApp (cmdlet commands) or Exchange.ManageAsAppV2 (the Admin API v2.0 commands) plus an Exchange role such as Exchange Administrator:

```bash
EXCHANGE_ONLINE_TOKEN=<value> exchange-online-cli doctor
```

On Windows (PowerShell):

```powershell
$env:EXCHANGE_ONLINE_TOKEN = "<value>"; exchange-online-cli doctor
```

`doctor` checks config, paths and API reachability, and tries the credential with one read cmdlet (`Get-AcceptedDomain`) when it can. Run the read commands you need to confirm the token's Exchange role covers them.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is any mail auto-forwarding out of this tenant, and does policy block it? | `exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --sweep --agent` |
| Which inbox rules forward, redirect, delete or hide mail, tenant-wide? | `exchange-online-cli inbox-rules sweep 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent` |
| Which mailboxes still have EWS, IMAP, POP or SMTP AUTH effectively on? | `exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --all --agent` |
| What compromise evidence does this mailbox show? | `exchange-online-cli bec triage 0550b473-9fd2-4dbb-a058-a1640b4bf458 alex@example.com --agent` |
| Which offboarding steps are done or pending for this leaver? | `exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --plan --agent` |
| Which mailboxes can this user open or send as? | `exchange-online-cli access-map 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --agent` |
| Why did this email not arrive? | `exchange-online-cli where-is 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient alex@example.com --since 48h --agent` |
| Which quarantine release requests are oldest across my tenants? | `exchange-online-cli quarantine queue --older-than 24h --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Exchange Online administration usually means a PowerShell session per customer tenant. This connector calls the same REST admin endpoints the ExchangeOnlineManagement v3 module uses, one tenant per call, so an AI agent gets JSON answers with no PowerShell runtime or session to keep alive.

On top of the raw cmdlets it adds compound commands that each answer a whole question: `forwarding-audit` (mailbox forwarding, inbox rules, the outbound spam policy and remote domains in one verdict), `bec triage` (rules, forwarding, restricted-sender status, new mobile devices and outbound volume for one mailbox), `ews-posture` (each mailbox's effective protocol state once org defaults apply) and `offboard` (the Exchange side of a leaver with a per-step receipt). `inbox-rules sweep` and `quarantine snapshot` save results locally, so `forwarding-audit` and the cross-tenant `quarantine queue` can read them without another pass. API responses themselves are not kept in a response cache on disk.

## The pain this closes

Microsoft began a phased shutdown of Exchange Web Services (EWS) in Exchange Online on October 1, 2026, with full retirement on April 1, 2027 (Microsoft 365 Message Center MC1227454). Every MSP has to find which client mailboxes still depend on EWS and the other legacy protocols, and the effective answer per mailbox depends on the org default and the mailbox override together. Meanwhile business email compromise keeps leaving inbox rules and forwarding behind, and those live per mailbox (Practical365, "Good Use of Microsoft 365 Auditing Can Halt Business Email Compromise Attempts").

- `ews-posture`: the mailboxes that break, or stay exposed, when EWS and legacy auth go away.
- `forwarding-audit` and `inbox-rules sweep`: whether mail is leaving a tenant, and which rule sends it.
- `bec triage`: one mailbox's compromise evidence in one call.
- `offboard --plan`: a leaver's Exchange steps, done or pending, before anything changes.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Microsoft Exchange Online MCP server runs locally over stdio by default. Its HTTP mode (`exchange-online-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `EXCHANGE_ONLINE_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Microsoft Exchange Online credentials once.

### Is my Microsoft Exchange Online data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The local store (inbox-rule sweeps and quarantine snapshots, with secret fields stripped; cmdlet reads are not saved) sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials come from your environment or the CLI's own credentials file (saved by `auth set-token`); they are never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I still need Exchange Online PowerShell?

Not for the cmdlets this connector covers: it calls the same REST admin endpoints the ExchangeOnlineManagement v3 module uses. Bulk policy creation, Purview compliance and Exchange Server on-premises stay with PowerShell or other tools.

### What token does it need?

A bearer token for the resource `https://outlook.office365.com`, issued for the customer tenant you are working in, with an Exchange role that allows the cmdlets you run. Set `EXCHANGE_ONLINE_TOKEN`, or save it with `exchange-online-cli auth set-token` (it reads the token from stdin). Every command names the tenant ID.

### Can an AI agent change mailboxes on its own?

Not over MCP without your approval: every state-changing cmdlet and `offboard` return a preview until the call carries `confirm: true`, and `EXCHANGE_ONLINE_MCP_READ_ONLY=1` removes every write tool. On the command line, preview a write with `--dry-run` (`offboard` with `--plan`) before you run it.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `mailboxes list`, `inbox-rules sweep` (removed over MCP in read-only mode), `forwarding-audit`, `ews-posture`, `bec triage`, `access-map`, `message-trace list`, `where-is`, `quarantine list` | Allow |
| Write (routine) | `mailboxes set`, `mailbox-permissions add`, `send-as add`, `send-on-behalf set`, `folder-permissions set`, `auto-reply set`, `client-access set`, `distribution-groups add-member`, `transport-rules enable`, `quarantine release`, `blocked-senders remove` | Preview with `--dry-run`, then a reviewed write (over MCP, `confirm: true`) |
| Destructive / config | `mobile-devices wipe`, `mobile-devices remove`, `quarantine delete`, `inbox-rules remove`, `mailbox-permissions remove`, `send-as remove`, `distribution-groups remove-member`, `folder-permissions remove`, `offboard`, `transport-rules disable` | Human-in-the-loop only |

The strongest control is the **scope you grant the Microsoft Exchange Online credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Microsoft Exchange Online API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
