# Mimecast + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Mimecast
> API. Not affiliated with, endorsed by, or sponsored by Mimecast Services Limited.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/mimecast/">
    <img src="../../docs/assets/video/mimecast/animated-og.gif" alt="Mimecast demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/mimecast/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Find a held or blocked Mimecast message, explain why in one verdict, and release it behind a safety check, with hold-queue triage, mail-flow health and phish click tracing built in. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Mimecast skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `mimecast-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `mimecast-mcp` over HTTPS with `MIMECAST_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `mimecast-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `mimecast-mcp --transport http` with `MIMECAST_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `mimecast-mcp` over HTTPS with `MIMECAST_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Mimecast MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/mimecast-v0.1.2/mimecast-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Mimecast release on the [releases page](https://github.com/servosity/msp-skills/releases?q=mimecast).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install mimecast@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Mimecast Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1 | iex`. Then authenticate per the README and run `mimecast-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh)
```

The installer drops both `mimecast-cli` (the CLI) and `mimecast-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
mimecast-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mimecast/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update mimecast@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/mimecast --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/mimecast --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `mimecast-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the mimecast skill from https://github.com/servosity/msp-skills/tree/main/skills/mimecast. The skill defines how its required CLI (`mimecast-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Mimecast API 2.0 uses OAuth2 client credentials. In the Mimecast Administration Console go to **Administration > Services > API and Platform Integrations > Mimecast API 2.0 > Generate Keys**, choose the products you need (Email Security Cloud Gateway, Audit Events, Security Events) and a least-privilege role, then set the client ID and secret. API 1.0 access keys do not work.

macOS / Linux:

```bash
export MIMECAST_CLIENT_ID=<your-client-id>
export MIMECAST_CLIENT_SECRET=<your-client-secret>
mimecast-cli doctor
```

Windows (PowerShell):

```powershell
$env:MIMECAST_CLIENT_ID = "<your-client-id>"
$env:MIMECAST_CLIENT_SECRET = "<your-client-secret>"
mimecast-cli doctor
```

The CLI exchanges them for a 30-minute bearer token and mints a new one automatically. For data residency, set both `MIMECAST_BASE_URL` and `MIMECAST_TOKEN_URL` to your regional gateway (for example `https://uk-api.services.mimecast.com` and `https://uk-api.services.mimecast.com/oauth/token`; `us-api` and `de-api` work the same way). With Mimecast delegate access, `MIMECAST_ACCOUNT_CODE` (or `--account-code` on the workflow commands) selects one customer account. `doctor` checks config, paths, and API reachability, and verifies the credential with one authenticated read (a 401 reads as rejected, a 404 as a wrong base URL).

## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Why didn't this message arrive: held, rejected, released or delivered, and by which policy? | `mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent` |
| What is in the hold queue, grouped by reason, with a release-risk class per group? | `mimecast-cli held triage --since 24h --group-by reason --agent` |
| Release this held message for the ticket, only if its hold reason is safe (preview first) | `mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan` |
| Who clicked this phishing link, and did anyone click through a warning? | `mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent` |
| Which users did this phishing sender reach, which clicked, which still have it held? | `mimecast-cli threats sender @phish.example --since 3d --agent` |
| Every threat, rejection and hold for one mailbox this week | `mimecast-cli threats mailbox alice@example.com --since 7d --agent` |
| Who released risky held messages this week, and which releases have no ticket? | `mimecast-cli held release-audit --since 7d --agent` |
| Is mail flowing? One OK/WARN/FAIL verdict | `mimecast-cli mailflow --max-queue 200 --stale-after 30m --agent` |

These workflow commands call the Mimecast API live, in one run. `why` reports a `stage` (held, rejected-at-gateway, released-from-hold, rejected-from-hold, the tracking status, or not-found) with the hold or reject reason, policy, spam score and SPF/DKIM/DMARC checks. `held triage` returns groups with a `risk_class` (never-release, likely-junk, policy-review, review) and ready-to-act `id_batches`. `threats mailbox` and `threats sender` report `fetch_failures` and `incomplete_sources`, so a short answer is never read as clean. `mailflow` exits non-zero on FAIL (and on WARN with `--fail-on-warn`), so an RMM monitor can run it. Every covered Mimecast API 2.0 endpoint is also a typed command (`held list`, `messages search`, `rejections`, `ttp url-logs`, ...).

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Mimecast integrations, such as the [Cortex XSOAR Mimecast pack](https://github.com/demisto/content/tree/master/Packs/Mimecast), expose the API as one action per endpoint, for a playbook or a person to chain. That works for one lookup. It is slow for the questions a service desk actually gets, where one answer needs message tracking, the hold queue, the held release log and the rejection log together.

This connector answers each of those questions in one command. `why` joins tracking, the hold queue, the release log and the rejection log into one verdict; `held triage` groups the whole hold queue by reason with a release-risk class; `held release-safe` checks the hold reason before it releases and records the ticket in a local ledger that `held release-audit` reads back; `threats sender` and `threats mailbox` join the URL Protect, Attachment Protect and Impersonation Protect logs with rejections and holds. `senders suggest` keeps a local cache of the release log, so permit and block suggestions build on weeks of history beyond any single API window.

## The pain this closes

"Release my email" tickets are urgent and risky at the same time. [Thread's public Mimecast runbook for MSP service desks](https://docs.getthread.com/skill-library/vendor-runbooks/mimecast-email-gateway) tells techs to verify the requester, treat urgency as a caution flag, never release a malware verdict or a sandbox or impersonation hold on the requester's word alone, treat an allowed click as an incident, and pull the click logs for sibling deliveries of the same URL. Each of those checks is a separate page in the Mimecast Administration Console, and the URL Protect log has no URL filter, so finding every clicker means paging the whole log.

- `mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent` - where the message is and why, in one verdict, instead of four console searches.
- `mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan` - the hold-reason check before a release: impersonation, malware, sandbox and policy holds are refused unless you give an override reason.
- `mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent` - every user who clicked a reported link, and who clicked through a warning.
- `mimecast-cli threats sender @phish.example --since 3d --agent` - the blast radius of one phishing sender: who it reached, who clicked, and which messages are still held.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Mimecast MCP server runs locally over stdio by default. Its HTTP mode (`mimecast-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `MIMECAST_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Mimecast credentials once.

### Is my Mimecast data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which Mimecast deployments and API version does it work with?

Mimecast API 2.0, with a client ID and secret from the Mimecast API 2.0 tile in the Administration Console. API 1.0 access keys do not work. The hold queue, `why`, `mailflow`, message tracking and TTP log commands are for MX-based (Cloud Gateway) deployments; the `remediation` and `ci-policies` commands are for Cloud Integrated tenants.

### Can an agent release a phishing email by accident?

`held release-safe` refuses impersonation, malware, sandbox and policy holds unless `--override` gives a reason, and `--plan` shows the decision without releasing anything; `held purge-sender` only plans until you add `--yes`. Over MCP, every tool that changes Mimecast returns a preview until it is called again with `confirm: true`, and `MIMECAST_MCP_READ_ONLY=1` makes the MCP server refuse every write. The raw `held release` and `held reject` commands send when you run them, so keep agents on `held release-safe --plan` and `--dry-run` previews.

### Does it work across my clients' Mimecast accounts?

With Mimecast delegate access, set `MIMECAST_ACCOUNT_CODE` (or pass `--account-code` on the workflow commands) to act on one customer account. It applies to the classic endpoints: hold queue, message tracking, logs and managed-sender permit or block. The REST endpoints (`gateway`, `journaling`, `ci-policies`, `remediation`, `senders bulk-delete`) have no account selector, so while an account code is set the CLI refuses their writes and warns on their reads. Delegation has not yet been verified against a live delegate tenant.

### Will it hit my Mimecast API rate limits?

Mimecast sets quotas per function and per API application. Read commands back off and retry on HTTP 429; writes (release, reject, permit or block) exit with code 7 instead of replaying, so rerun them after the wait. The workflow commands page the logs only up to `--max-scan-pages` pages (100 entries each) and say when a result was truncated.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `why`, `held triage`, `held list`, `held release-logs`, `held release-audit`, `senders suggest`, `mailflow`, `ttp who-clicked`, `threats mailbox`, `threats sender`, `messages search`, `rejections`, `audit events`, `archive-logs searches` | Allow |
| Write (routine) | `held release-safe`, `held release`, `held reject`, `senders permit-or-block`, `ci-policies create`, `ci-policies update`, `ci-policies update-default`, `files`, `import` | Preview first (`held release-safe --plan`; `--dry-run` elsewhere; MCP `confirm`), then a reviewed write |
| Mail flow and sending | `email`, `gateway set-protection-mode`, `gateway set-outbound-ips`, `gateway update-details` | Human-in-the-loop only |
| Credential / identity | any command that can emit a stored secret (`journaling get` returns journaling passwords, so its whole answer is withheld from output when it carries one; `messages get-file` returns a pre-signed download link), `journaling create`, `journaling update`, `auth login`, `auth set-token`, `auth logout` | Human-in-the-loop only |
| Destructive | `held purge-sender`, `remediation remediate`, `remediation bulk`, `ci-policies delete`, `gateway delete-outbound-ips`, `journaling delete`, `senders bulk-delete`, `senders bulk-delete-for-recipient`, `senders bulk-delete-internal`, `local-data purge` | Human-in-the-loop only, explicit confirmation |
| Admin | `ci-onboarding` (onboards a Cloud Integrated customer account) | Operator-only, not for agents |

The strongest control is the **scope you grant the Mimecast credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Mimecast API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-11-04._
