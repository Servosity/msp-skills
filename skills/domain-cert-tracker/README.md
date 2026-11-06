# Domain and Certificate Tracker + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of
> Servosity Inc. Apache-2.0 licensed. A first-party Claude Code Skill and MCP server
> that reads public RDAP, Certificate Transparency (crt.sh), DNS and TLS; not
> affiliated with any registry, registrar or CA.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/domain-cert-tracker/">
    <img src="../../docs/assets/video/domain-cert-tracker/animated-og.gif" alt="Domain and Certificate Tracker demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/domain-cert-tracker/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every client's domains and certificates in one inventory: what expires, unlocks or changes. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the domain-cert-tracker skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `domain-cert-tracker-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `domain-cert-tracker-mcp` over HTTPS with `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `domain-cert-tracker-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `domain-cert-tracker-mcp --transport http` with `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `domain-cert-tracker-mcp` over HTTPS with `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download the Domain and Certificate Tracker MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/domain-cert-tracker-v0.1.0/domain-cert-tracker-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every domain-cert-tracker release on the [releases page](https://github.com/servosity/msp-skills/releases?q=domain-cert-tracker).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install domain-cert-tracker@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the domain-cert-tracker Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex`. No credentials are needed; run `domain-cert-tracker-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)
```

The installer drops both `domain-cert-tracker-cli` (the CLI) and `domain-cert-tracker-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
domain-cert-tracker-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update domain-cert-tracker@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/domain-cert-tracker --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/domain-cert-tracker --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `domain-cert-tracker-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the domain-cert-tracker skill from https://github.com/servosity/msp-skills/tree/main/skills/domain-cert-tracker. The skill defines how its required CLI (`domain-cert-tracker-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Nothing to set up: every source this skill reads is public, so there is no account, API key or token. The only optional variable is `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN`, which protects the MCP server's HTTP mode (see [mcp-install.md](./mcp-install.md)).


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What expires within 30 days, is not transfer-locked or changed, per client? | `domain-cert-tracker-cli report --days 30 --agent` |
| Which hosts still serve an old certificate after a newer one was logged? | `domain-cert-tracker-cli renewals --agent` |
| Which new certificates came from an issuer the domain's CAA does not permit, or a first-ever issuer? | `domain-cert-tracker-cli issuers audit --days 7 --agent` |
| Which hostnames have certificates in CT but are not in the client's inventory? | `domain-cert-tracker-cli hosts discover --agent` |
| What changed since the agent last looked, once per event? | `domain-cert-tracker-cli events list --since 0 --agent` |
| What certificate does this host serve right now? | `domain-cert-tracker-cli tls probe www.example.com --agent` |
| Who is the registrar, when does the domain expire, is it locked? | `domain-cert-tracker-cli domain example.com --json` |

Start with `domain-cert-tracker-cli clients set "Acme Dental" --domain example.com --host www.example.com --yes`, then `domain-cert-tracker-cli sweep --agent` (or schedule it daily); every question above except `tls probe` (a live handshake) then reads the local store.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most domain and certificate MCP servers answer one lookup at a time: one RDAP query, one crt.sh search, one TLS handshake. That's fine for one domain. It dies at scale, when you're asking "which of my 40 clients' certificates were renewed but never deployed?"

This skill keeps a per-client inventory and a **local SQLite history**. `sweep` checks registrations, Certificate Transparency, live TLS and DNS for every inventoried domain and host, stores each result and records what changed as typed events. `renewals` joins each host's served certificate with the same-CA certificates logged in CT; `issuers audit` judges each certificate name against the CAA that applies to it; `report` puts expiry, lock, CT and DNS changes per client on one page - work a stateless lookup tool can't do.

## The pain this closes

Certificate lifetimes are shrinking: CA/Browser Forum ballot [SC-081v3](https://cabforum.org/2025/04/11/ballot-sc081v3-introduce-schedule-of-reducing-validity-and-data-reuse-periods/) cuts them to 200 days from March 2026, 100 days from March 2027 and 47 days from March 2029. Renewal becomes a monthly automated job across every client, and automation fails quietly: a renewed certificate that never reached the server, a certificate nobody ordered, a registration that lost its lock. MSPs still ask for a [free SSL expiration tracker that makes sense](https://www.reddit.com/r/msp/comments/1ohanfn/free_ssl_expiration_tracker_that_makes_sense/) on r/msp.

- `domain-cert-tracker-cli report --days 30 --agent` - what expires, unlocked or changed, per client.
- `domain-cert-tracker-cli renewals --agent` - renewals issued but never deployed.
- `domain-cert-tracker-cli issuers audit --days 7 --agent` - certificates from an unexpected issuer.
- `domain-cert-tracker-cli events list --since 0 --agent` - each change once, ready for a ticket.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The domain-cert-tracker MCP server runs locally over stdio by default. Its HTTP mode (`domain-cert-tracker-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `DOMAIN_CERT_TRACKER_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. There are no credentials to enter.

### Is my client data safe?

Your inventory stays on **your machine**. The CLI and MCP server are local binaries, and the SQLite store sits in a directory under your user account. A lookup sends only the domain or host being checked to the public service that answers for it. HTTP lookups and TLS probes connect only to public addresses, on port 443 unless you pass `--allow-port` on the command line (the one exception is a server you name yourself in `DOMAIN_CERT_TRACKER_BASE_URL`); DNS checks use the resolver your operating system is configured with.

### Do I need an account or API key?

No. RDAP, crt.sh and DNS are public, and TLS probes connect to your clients' own public hosts. There is nothing to sign up for and no credential to store.

### Will it hit rate limits?

`sweep` sends each RDAP lookup straight to the registry that answers for the domain (through the IANA bootstrap file) and runs at most 4 checks in parallel. It also downloads at most 50 new certificates per sweep from crt.sh, once each, to read their full name lists. crt.sh is a free shared service: when it fails or cuts an answer off at its 10,000-row limit, the domain is listed as unverified and its last good result is kept. Run `sweep` again later.

### Can it renew or change anything?

No. It never contacts a registrar account, a CA account, a DNS provider or a server's configuration. The only things it writes are its own local inventory and history.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `report`, `renewals`, `issuers audit`, `hosts discover`, `events list`, `clients list`, `domain`, `tls probe`, `dns check`, `certificates` | Allow |
| Write (routine, local store only) | `sweep` | Allow; it records only this tool's own sweep history |
| Write (inventory, local store only) | `clients set` (previews unless `--yes` or `confirm: true`), `clients import` (command line only) | Allow after confirm; adds to the inventory every sweep checks |
| Destructive (local store only) | `clients remove` (previews unless `--yes` or `confirm: true`; a whole client only with `--whole-client`), `local-data purge` (dry run unless `--yes`) | Human-in-the-loop only |

There are no credentials to scope: nothing this skill does can change a registration, a certificate or a DNS record. The controls that matter are the outbound guard (public addresses, port 443 unless the command line allows another) and the confirm step on `clients set` and `clients remove`. Full details are in [governance.md](./governance.md).

## Status

Beta. Tested against the public RDAP, crt.sh, DNS and TLS services it reads, and being validated with MSPs running it on their own client lists in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
