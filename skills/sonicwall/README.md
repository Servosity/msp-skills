# SonicWall + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the SonicWall
> API. Not affiliated with, endorsed by, or sponsored by SonicWall Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/sonicwall/">
    <img src="../../docs/assets/video/sonicwall/animated-og.gif" alt="SonicWall demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/sonicwall/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Cyber-insurance posture for a whole SonicWall fleet: firmware, advisories, MFA, SSL-VPN exposure and licences, one row per firewall. Works with the AI you already use - **ChatGPT** (Plus/Pro+, paused until this connector is reprinted; see [mcp-install.md](./mcp-install.md)), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the SonicWall skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `sonicwall-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Paused until this connector is reprinted: ChatGPT needs its HTTP mode, and in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md). |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `sonicwall-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

ChatGPT needs a remote HTTPS server, which means this connector's HTTP mode, and that is paused until the connector is reprinted: in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | Paused until this connector is reprinted: Copilot Studio needs its HTTP mode, and in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote and needs the same paused HTTP mode as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download SonicWall MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/sonicwall-v0.1.3/sonicwall-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every SonicWall release on the [releases page](https://github.com/servosity/msp-skills/releases?q=sonicwall).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install sonicwall@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the SonicWall Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex`. Then authenticate per the README and run `sonicwall-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)
```

The installer drops both `sonicwall-cli` (the CLI) and `sonicwall-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
sonicwall-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update sonicwall@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/sonicwall --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/sonicwall --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sonicwall-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sonicwall skill from https://github.com/servosity/msp-skills/tree/main/skills/sonicwall. The skill defines how its required CLI (`sonicwall-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create a MySonicWall API key (ideally for an account whose NSM role is read-only), then mint the NSM session token from it and check it:

```bash
export SONICWALL_MSW_API_KEY=<your MySonicWall API key>
export SONICWALL_REGION=eucentral   # EU NSM portals only; the default is uswest
sonicwall-cli nsm-login
sonicwall-cli doctor
```

`nsm-login` trades the key for an NSM session token and saves it (never printed). The token lasts 8 hours with a 15-minute idle timeout, so run `nsm-login` again whenever NSM commands exit 4. `doctor` then makes one authenticated NSM read (`GET /tenants`), so it reports whether the token actually works, not just whether one is set. Licence checks and `blind-spots` also read MySonicWall with the same key.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| One insurance-posture row per client firewall: firmware, support, advisories, licences, admin TOTP, SSL-VPN | `sonicwall-cli posture` |
| Which firewalls does the CVE-2024-40766 advisory hit, and which have SSL-VPN on a WAN zone? | `sonicwall-cli exposure "SNWLID-2024-0015"` |
| Which SonicWall advisories hit the most firewalls in my fleet? | `sonicwall-cli exposure` |
| Which admins and local SSL-VPN users have no one-time password? | `sonicwall-cli mfa-audit` |
| Which firewalls are behind the newest firmware, and what does the upgrade clear? | `sonicwall-cli firmware drift --behind-only` |
| Which licences expire in the next 90 days, and which boxes should be replaced instead? | `sonicwall-cli licenses renewals --days 90` |
| Which firewalls are registered in MySonicWall but invisible to NSM? | `sonicwall-cli blind-spots` |
| What got worse since last week? | `sonicwall-cli posture changes --fleet --since 7d` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

[GTalksTech/sonicwall-mcp](https://github.com/GTalksTech/sonicwall-mcp) is a read-only Python MCP server for NSM SaaS with 46 tools for inventory, firmware, advisories, licence posture, per-site SSL-VPN configuration and config history.

This one is a single binary that works as a CLI and an MCP server, built for the questions an MSP gets the morning a SonicWall advisory lands and again at insurance renewal. `posture` reads every firewall in your NSM tenant through NSM's SonicOS passthrough and grades each one pass, warn, fail or unknown for firmware, end of support, advisories, licence expiry, admin TOTP and SSL-VPN exposure. `mfa-audit` names every local SSL-VPN user without a one-time password, `blind-spots` finds firewalls registered in MySonicWall that NSM cannot see, and each `posture` run is saved locally so `posture changes` shows what got worse since last week. A firewall it could not read is unknown, never pass. Secret fields SonicWall's own API schemas document (local-user and device passwords, registration and authentication codes, SNMP community names, MySonicWall API keys) are blanked before anything is printed, cached or handed to an agent.

SonicWall's [SAMI](https://www.sonicwall.com/blog/the-firewall-is-not-dead-how-ai-gives-it-a-co-pilot) is an assistant inside SonicWall's Unified Management console that answers fleet questions and can run tasks such as firmware upgrades. This works from the AI agent you already use, never changes a firewall, and grades the posture an insurer asks about per firewall.

## The pain this closes

From July 2025 Akira ransomware operators went after SonicWall SSL-VPN accounts ([BleepingComputer](https://www.bleepingcomputer.com/news/security/surge-of-akira-ransomware-attacks-hits-sonicwall-firewall-devices/)). SonicWall linked the wave to CVE-2024-40766 and to Gen 6 to Gen 7 migrations "where local user passwords were carried over", and told admins to update firmware to 7.3.0, reset local SSL-VPN passwords and enforce MFA ([SonicWall](https://www.sonicwall.com/support/notices/gen-7-and-newer-sonicwall-firewalls-sslvpn-recent-threat-activity/250804095336430)); the MSP-facing advisory from [SmarterMSP](https://smartermsp.com/cybersecurity-threat-advisory-sonicwall-ssl-vpn-targeted-by-akira-ransomware/) said the same. Doing that the same day means knowing, for every client, which firewalls run old firmware, which have SSL-VPN on a WAN zone and which local users have no one-time password. Those settings live on each firewall, and insurers ask the same questions every renewal.

- `sonicwall-cli exposure "SNWLID-2024-0015"` - every firewall the CVE-2024-40766 advisory hits, and whether SSL-VPN is on a WAN zone.
- `sonicwall-cli mfa-audit` - admin TOTP per firewall and every local SSL-VPN user without a one-time password.
- `sonicwall-cli posture` - one pass, warn, fail or unknown row per firewall, saved for `posture changes`.
- `sonicwall-cli licenses renewals --days 90` - one renewal list instead of a spreadsheet.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Not with this build. ChatGPT (Plus, Pro, Team, Business, Enterprise, and Education plans; Free does not yet expose Developer Mode) connects only to **remote** MCP servers over HTTPS, which means running the SonicWall MCP server in HTTP mode. In this build HTTP mode checks a token but has no request size or time limits yet, so do not run it or put it behind a tunnel until the connector is reprinted; the security note in [mcp-install.md](./mcp-install.md) explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter a MySonicWall API key once and run `sonicwall-cli nsm-login`.

### Is my SonicWall data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What credentials does it need?

A MySonicWall API key, set as `SONICWALL_MSW_API_KEY`. `sonicwall-cli nsm-login` trades it for an NSM session token and saves it; the token lasts 8 hours with a 15-minute idle timeout, so run `nsm-login` again when NSM commands exit 4. EU portals also set `SONICWALL_REGION=eucentral`. Use a MySonicWall account whose NSM role is read-only, and NSM itself refuses any write.

### Can it change my SonicWall firewalls?

No. It reads NSM, the firewalls through NSM's SonicOS passthrough, and MySonicWall. The only POSTs are read-style MySonicWall queries and the token exchanges (`nsm-login`, `nsm-session`, `msw-tenants access-code`). Secret fields SonicWall's own API schemas document, such as local-user and device passwords, registration and authentication codes, SNMP community names and MySonicWall API keys, are blanked in every response before anything is printed, cached, synced or handed to an agent. `SONICWALL_MCP_READ_ONLY=1` hides the MCP tools that could change NSM, MySonicWall or firewall state; the tools it keeps still write local state (the response cache, `posture` snapshots and learning entries).

### Will it see firewalls that are not in NSM?

Not with `posture`, `exposure`, `mfa-audit` or `firmware drift`: those only see firewalls onboarded to NSM SaaS. Run `sonicwall-cli blind-spots` to list firewalls registered in MySonicWall that NSM cannot see, with the advisories that apply to their registered firmware.

### How does it decide SSL-VPN is exposed to the internet?

It reads each firewall's SSL-VPN zone settings and marks SSL-VPN as exposed when it is enabled on a zone whose name contains WAN. An internet-facing zone with a custom name that does not contain WAN is missed, so treat a pass there as "not on a WAN-named zone".

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `posture`, `posture changes`, `exposure`, `mfa-audit`, `firmware drift`, `licenses renewals`, `blind-spots`; every list / get / count under `devices`, `device-groups`, `firmware`, `licenses`, `vulnerabilities`, `tenants`, `alerts` and `nsm-system`; `config-audit count`; the `firewall` reads other than the four under Sensitive read; `msw-products`, `msw-licenses`, `msw-system` and the `msw-tenants` reads other than `access-code`; `search` over the local store, and `export` of any resource other than `config_audit` and `msw_users` (local writes only: saved posture runs, the response cache, an `export --output` file) | Allow |
| Raw read (escape hatch) | `nsm-get` (vendor-documented GET routes only; credential and state-changing routes refused; credential-shaped values redacted) | Allow; review unfamiliar paths |
| Sensitive read | `config-audit list` / `diff` (configuration text as NSM returns it, not redacted field by field), `firewall local-users`, `firewall local-groups`, `firewall user-auth`, `firewall sslvpn-sessions`, `msw-users`, `msw-activity`, `export config_audit` / `export msw_users`, `sync` / `workflow archive` (they copy config-audit configuration text and firewall local users and groups into the local store, where `search` and the MCP `sql` / `search` tools read them), and `tail` of `config_audit` or `msw_users` | Human-in-the-loop; keep output and the local store private |
| Credential | `nsm-login`, `nsm-session refresh` / `sso`, `msw-tenants access-code`, `import`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| Data egress | `--deliver webhook:<url>`, `feedback --send`, and `feedback` when `SONICWALL_FEEDBACK_ENDPOINT` is set with `SONICWALL_FEEDBACK_AUTO_SEND=true` | Human-in-the-loop |

The MCP `sonicwall_execute` tool can call any curated endpoint by ID: each call follows the tiers above. Its POSTs are the Credential-tier token exchanges plus four read-style MySonicWall queries (`msw-activity export`, `msw-licenses`, `msw-products list`, `msw-users groups`), and every POST is refused while `SONICWALL_MCP_READ_ONLY=1`. No command changes a firewall, NSM or MySonicWall configuration.

The strongest control is a **MySonicWall account whose NSM role is read-only** - NSM then refuses anything else. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta, awaiting live verification. The command surface is a curated subset of SonicWall's own published API specifications (the NSM API Gateway and SonicOS passthrough Swagger files, and the MySonicWall API), and the fleet commands are covered by unit tests against simulated NSM and MySonicWall responses, but no MSP has yet run it against a production tenant and reported back - so nothing here is proven against real firewalls. If you run it against your own, the it-works form is how that becomes a badge. MSPs put connectors like this through their paces in our weekly Build Sessions - RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-15._
