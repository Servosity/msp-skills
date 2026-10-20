# SaaS Alerts + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the SaaS Alerts
> API. Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/saas-alerts/">
    <img src="../../docs/assets/video/saas-alerts/animated-og.gif" alt="SaaS Alerts demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/saas-alerts/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

All four SaaS Alerts APIs (Reports, Manage, Respond and Webhooks) in one CLI and MCP server, plus cross-customer alert triage, the Respond approval queue sorted by deadline, and preview-first account containment. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the SaaS Alerts skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `saas-alerts-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `saas-alerts-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `saas-alerts-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `saas-alerts-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `saas-alerts-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download SaaS Alerts MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/saas-alerts-v0.1.0/saas-alerts-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every SaaS Alerts release on the [releases page](https://github.com/servosity/msp-skills/releases?q=saas-alerts).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install saas-alerts@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the SaaS Alerts Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex`. Then authenticate per the README and run `saas-alerts-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh)
```

The installer drops both `saas-alerts-cli` (the CLI) and `saas-alerts-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
saas-alerts-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update saas-alerts@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/saas-alerts --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/saas-alerts --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `saas-alerts-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the saas-alerts skill from https://github.com/servosity/msp-skills/tree/main/skills/saas-alerts. The skill defines how its required CLI (`saas-alerts-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the partner API key from the SaaS Alerts dashboard (**Settings > API**, **GET APIKEY**; SaaS Alerts shows the key only until you leave the page, so copy it then):

```bash
SAAS_ALERTS_API_KEY=<value> saas-alerts-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SAAS_ALERTS_API_KEY = "<value>"
saas-alerts-cli doctor
```

`doctor` checks config, paths and API reachability, and sends the key to the Reports API's partner profile endpoint, so a rejected key shows up as a `Credentials: invalid` row. It exits 0 even then, so scripts should add `--fail-on error`. The Respond access token is minted on the first Respond call, not by `doctor`; `saas-alerts-cli respond status` confirms Respond works end to end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What fired across every customer in the last 24 hours, critical first? | `saas-alerts-cli triage --since 24h --min-severity medium` |
| Which Respond triggers expire or auto-ignore soonest? | `saas-alerts-cli rule-triggers queue --expiring-within 2h` |
| Is one IP address hitting two or more of my customers? | `saas-alerts-cli cross-tenant --by ip --since 24h --min-customers 2` |
| Everything on this one account: events, country hops, Respond triggers, devices | `saas-alerts-cli investigate "jane.doe@example.com" --since 7d` |
| Which customers went silent, lost their Respond connection or have no Respond rules? | `saas-alerts-cli coverage --silent 72h` |
| Why did SaaS Alerts tickets stop arriving? Is a webhook disabled? | `saas-alerts-cli subscriptions health` |
| Reject every pending trigger from this noisy rule (preview first) | `saas-alerts-cli rule-triggers decide --decision reject --rule <rule-id> --remarks "false positive"` |
| Which inbox rules exist at this customer? | `saas-alerts-cli reports mailbox-rules --organization-ids <customer-id>` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most SaaS Alerts integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 80 customers had a critical alert overnight, and is the same IP behind more than one of them".

This skill syncs customers, events, users and Respond triggers into a **local SQLite store**, so `triage`, `cross-tenant`, `investigate` and `coverage` answer across every customer in one local query, and say `synced: false` instead of an empty all-clear when the data was never synced. It covers all four SaaS Alerts APIs with one partner key: it mints the Respond access token itself (Respond answers "Only secure origins are allowed" to a call that carries only the API key), works the Respond queue by deadline with `rule-triggers queue`, and previews every change `rule-triggers decide` and `contain` would make until you add `--confirm`.

[WYRE-AI's saas-alerts-mcp](https://github.com/WYRE-AI/saas-alerts-mcp), which also backs the SaaS Alerts plugin in their msp-claude-plugins marketplace, exposes the SaaS Alerts External Partner API as 30 tools across customers, events, users, devices, billing, scheduled reports and partner settings, with no Respond, suppression or webhook tools; it runs locally with your own key, or through WYRE's hosted Conduit gateway when used from their plugin. This one adds Respond, suppressions and webhooks. It complements the SaaS Alerts dashboard rather than replacing it: the dashboard stays the place to configure monitoring, review alerts and set up Respond.

## The pain this closes

SaaS Alerts' Respond module is on a clock. The Respond API documents that a manual approval "will auto expire when the rule triggers again or 12 hours whichever is earlier", and the SaaS Alerts help center ([Analyzing a Respond trigger](https://help.saasalerts.kaseya.com/help/Content/How-To/analyzing-a-respond-trigger.htm)) says a failed remediation is reminded every 15 minutes and automatically ignored after the third reminder. Across many customers, the hard part is knowing which decision lapses first. Scripts that try to help hit a second credential: Respond wants a short-lived access token minted from the partner key, and a call without it fails with "Only secure origins are allowed". In the r/msp thread ["Kaseya Saas Alerts API"](https://www.reddit.com/r/msp/comments/1p9dbc1/kaseya_saas_alerts_api/) an MSP got that error on every call, another replied they had been "banging my head against this same issue for weeks", suspecting CORS, and the fix the thread landed on was to POST to the auth endpoint first and send the access token. And the alert volume is real: MSP reviewers of SaaS Alerts on [G2](https://www.g2.com/products/saas-alerts/reviews) describe a flood of notifications until the configuration is tuned and false positives you learn to filter out over time.

- `saas-alerts-cli triage --since 24h` - one severity-ordered digest across every customer, grouped by customer, account and event type.
- `saas-alerts-cli rule-triggers queue --expiring-within 2h` - pending and failed Respond triggers by how soon they expire or auto-ignore, with the actions an approval would fire.
- `saas-alerts-cli cross-tenant --by ip` - a source address, network or country hitting two or more customers, minus each customer's own whitelist.
- `saas-alerts-cli coverage` - customers that went silent, lost their Respond connection or have no Respond rules.
- `saas-alerts-cli contain "jane.doe@example.com"` - block sign-in, expire sessions and reset the password for one Microsoft 365 account, previewed first.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The SaaS Alerts MCP server runs locally, but it speaks HTTP natively: start it with `saas-alerts-mcp --transport http` and `SAAS_ALERTS_MCP_HTTP_TOKEN` set (it serves `http://127.0.0.1:7777/mcp` by default and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header (ChatGPT connectors do not send a fixed one). No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your SaaS Alerts credentials once.

### Is my SaaS Alerts data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite store sits in a directory under your user account and holds your customers' account emails, sign-in IPs and event details, so treat it like any other copy of that data. Each webhook subscription's delivery token is withheld before it is printed, cached or stored. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Where do I get the SaaS Alerts API key?

In the SaaS Alerts dashboard, open Settings > API and click GET APIKEY. SaaS Alerts shows the key only until you leave that page, so copy it then. Set it as SAAS_ALERTS_API_KEY. That one partner key reaches the Reports, Manage and Webhooks APIs, and the CLI exchanges it for the Respond access token on first use, so there is no second credential to paste. If every call answers 401 Access denied, ask SaaS Alerts support to enable the REST API for your account.

### Why does the Respond API answer "Only secure origins are allowed"?

That is what Respond returns when a call carries the api_key but no access token. Respond needs a short-lived token minted from the partner key with POST /respondApi/api/v1/auth. This connector mints it on the first Respond call, sends it on every Respond request, keeps it in memory for that process only, and mints a fresh one once if Respond rejects it.

### Can the AI block a user or approve Respond actions by itself?

Only if you let it. contain, rule-triggers decide and subscriptions health --reenable preview by default and change nothing until you add --confirm. The generated commands (rule-triggers approve, response-actions execute, customers delete and the rest) send as soon as they run, so preview them with --dry-run, which sends nothing. Over MCP, set SAAS_ALERTS_MCP_READ_ONLY=1 for an agent that should only read: saas-alerts_execute then runs GET endpoints only and the three workflow writers refuse --confirm.

### Do Respond triggers expire?

Yes. The Respond API documents that a manual approval expires after 12 hours or when the rule triggers again, whichever is earlier, and the SaaS Alerts help center says a failed remediation is automatically ignored after the third 15-minute reminder. rule-triggers queue lists pending and failed triggers by that deadline, with the actions an approval would fire, and rule-triggers decide skips expired or superseded triggers unless you pass --include-expired. The API does not say whether "triggers again" means anywhere or for the same customer, so the CLI counts a trigger as superseded only by a newer one of the same rule for the same organization and accounts: it may show a trigger Respond already expired (Respond then refuses the approve), but it never hides a live one.

### Why does triage say synced: false?

triage, investigate, cross-tenant and coverage read the local store, so they need a sync first, for example `saas-alerts-cli sync --resources customers,events --since 7d`. They say synced: false, with a note naming the fix, when the data they need was never synced, when the last events sync did not finish (an automatic refresh fetches one page at a time), or when the window you asked about starts before the oldest events you synced. An events sync narrowed with --param stores its rows but does not count as complete. An empty or partial result is never presented as an all-clear, and --select always keeps the synced and note fields.

### Does contain work for Google Workspace accounts?

No. Respond response actions run against Microsoft 365 only, so contain refuses a Google Workspace or other non-Microsoft account, and stops when an email matches more than one Microsoft 365 account. The API returns no job id for an action, so contain reports each job match as best-effort and always says verified: false; check the account in the Microsoft 365 admin center.

### Has it been tested against a live SaaS Alerts tenant?

Not yet. Every endpoint traces to SaaS Alerts' published API specs and the Respond token exchange was checked against the live API without a key, but no partner has run it against real data yet. Two things wait on a live tenant: a partially rejected sync is reported as a sync_warning, so sync --strict can still exit 0 when some pages were refused; and the Unify device lists page with searchAfter from the response's meta.sort as the Manage spec describes, but if a tenant returns a bare list instead, a partner with more than 1,000 devices cannot sync them completely (sync and devices list --all then say truncated, never complete). Run it, and tell us what you find.

### Does it replace the SaaS Alerts dashboard?

No. The dashboard stays best for configuring monitoring, reviewing individual alerts and setting up Respond. This skill answers the across-every-customer questions and runs reviewed actions from your AI agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `triage`, `investigate`, `cross-tenant`, `coverage`, `rule-triggers queue`, `subscriptions health`, `rule-triggers decide` and `contain` (without `--confirm`), `customers list`, `events list`, `events query`, `users`, `reports customers`, `reports mfa-microsoft`, `reports mailbox-rules`, `rules list`, `rule-triggers list`, `suppressions list`, `subscriptions list` | Allow |
| Account response | `contain --confirm`, `rule-triggers decide --confirm`, `rule-triggers approve`, `rule-triggers reject`, `rule-triggers ignore`, `rule-triggers manually-remediate`, `response-actions execute` | Human-in-the-loop, explicit confirmation. These lock real people out. |
| Detection and Respond config | `customers set-whitelists`, `customers set-account-whitelists`, `suppressions create`, `suppressions update`, `suppressions delete`, `rules update`, `rules draft`, `rules duplicate`, `rule-templates use`, `iocs update`, `respond enable`, `application-connections connect`, `subscriptions create`, `subscriptions update`, `subscriptions health --reenable --confirm`, `webhook-domains verify` | Preview with `--dry-run`, then a human-approved write. The whitelist commands replace the customer's list, so read it first. |
| Write (routine) | `devices map`, `devices unmap`, `psa map`, `psa map-suggested`, `customers create`, `customers update`, `reports scheduled-create`, `reports update-branding`, `subscriptions test-event`, `files upload` | Preview with `--dry-run`, then a reviewed write |
| Credential and access | `files url` (returns a signed link to the stored file), `auth set-token`, `auth logout` | Human-in-the-loop only |
| Destructive and outbound | `respond disable`, `customers delete`, `rules delete`, `application-connections delete`, `subscriptions delete`, `webhook-domains delete`, `webhook-queue delete`, `reports scheduled-delete`, `files delete`, `reports send-email`, `import` | Human-in-the-loop only, explicit confirmation |

`rule-triggers decide`, `contain` and `subscriptions health --reenable` preview until you add `--confirm`; every other write runs as soon as it is invoked, so preview with `--dry-run`. SaaS Alerts documents no read-only or per-customer key, so the strongest control is the MCP read-only switch: set `SAAS_ALERTS_MCP_READ_ONLY=1` and `saas-alerts_execute` runs GET endpoints only while the three workflow writers refuse `--confirm`. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Built and tested against SaaS Alerts' published API specs and local mock servers; not yet run against a live SaaS Alerts tenant. Be the first: bring it to our weekly Build Sessions and run it against your own tenant. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-20._
