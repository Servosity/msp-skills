# WatchGuard + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the WatchGuard
> API. Not affiliated with, endorsed by, or sponsored by WatchGuard Technologies, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/watchguard/">
    <img src="../../docs/assets/video/watchguard/animated-og.gif" alt="WatchGuard Cloud demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/watchguard/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

WatchGuard Cloud's Firebox, FireCloud, Endpoint Security, ThreatSync, NDR, AuthPoint and account platform APIs in one CLI and MCP server, with cross-customer reports for Service Providers. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the WatchGuard skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `watchguard-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `watchguard-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `watchguard-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `watchguard-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `watchguard-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download WatchGuard MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/watchguard-v0.1.2/watchguard-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every WatchGuard release on the [releases page](https://github.com/servosity/msp-skills/releases?q=watchguard).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install watchguard@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the WatchGuard Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex`. Then authenticate per the README and run `watchguard-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)
```

The installer drops both `watchguard-cli` (the CLI) and `watchguard-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
watchguard-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update watchguard@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/watchguard --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/watchguard --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `watchguard-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the watchguard skill from https://github.com/servosity/msp-skills/tree/main/skills/watchguard. The skill defines how its required CLI (`watchguard-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your WatchGuard portal):

```bash
WATCHGUARD_ACCOUNT_ID=<value> WATCHGUARD_API_KEY=<value> WATCHGUARD_CLIENT_ID=<value> WATCHGUARD_CLIENT_SECRET=<value> watchguard-cli doctor
```

`doctor` checks config, paths and API reachability, and with the Access ID, password and API key set it requests a token and reads your account record, so a wrong password shows up as a `Credentials: ERROR` row (while a token minted earlier with the right password is still cached, doctor reuses it for up to an hour; run `watchguard-cli auth logout` first to recheck the password). It exits 0 even then, so scripts should add `--fail-on error`. A read command such as `watchguard-cli accounts list-managed --agent` is the end-to-end check.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customer needs attention first across Firebox, endpoints, ThreatSync and licenses? | `watchguard-cli accounts health --agent` |
| Which endpoints are unprotected, out of date, isolated or waiting on a reboot, across every customer? | `watchguard-cli endpoint-devices posture --agent` |
| Which new ThreatSync incidents from the last 30 days need triage first, and which has nobody touched? | `watchguard-cli threatsync-incidents queue --status new --duration 30D --agent` |
| Which customers and computers are exposed to this CVE? | `watchguard-cli endpoint-patches exposure --cve CVE-2024-38063 --agent` |
| Which Fireboxes and access points went offline, came back or changed firmware since the last sync? | `watchguard-cli firebox-devices changes --agent` |
| Which Fireboxes run older firmware than the rest of the same model in my fleet? | `watchguard-cli firebox-devices firmware-lag --agent` |
| Which allocations don't match the Fireboxes that actually report in, and what expires this month? | `watchguard-cli allocations reconcile --expiring-within 30d --agent` |
| Did the Firebox deployment finish, or fail? | `watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m` |

`accounts health`, `endpoint-devices posture` and `endpoint-patches exposure` are built for a WatchGuard Service Provider account: they reach each customer with its own token. `firebox-devices changes` and `firmware-lag` read the local copy, so sync the device list first: `watchguard-cli sync --resources firebox_devices --full` (it includes your customers' Fireboxes by default). `firebox-transactions wait` exits 0 when the deployment completes, 8 when it fails, times out or is canceled, and 9 when it is still running at `--max-wait`.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

The only other MCP server for WatchGuard Cloud we found on GitHub as of October 2026, [WYRE-AI/watchguard-cloud-mcp](https://github.com/WYRE-AI/watchguard-cloud-mcp), covers the platform's account, activation and operator APIs and lists Firebox and Endpoint Security as out of scope for now. A question like "which of my 40 customers has an offline Firebox, an unprotected laptop and an untouched incident" still means switching into every account.

This skill covers 208 operations from 13 of WatchGuard's published API specifications: Firebox Management and Reports, FireCloud, Endpoint Security, ThreatSync, NDR assets and Smart Alerts, AuthPoint authentication, and the platform's accounts, operators, activations, license allocation and authorization APIs. Where WatchGuard requires it, the cross-customer commands mint a separate token for each managed account, so one Service Provider credential reaches every customer, and it syncs WatchGuard into a **local SQLite mirror** with full-text search. Compound commands like `accounts health`, `allocations reconcile` and `firebox-devices changes` join Fireboxes, endpoints, incidents, allocations and earlier snapshots of your fleet - work a stateless API wrapper can't do. WatchGuard Cloud stays where you configure Fireboxes and policies, and ThreatSync keeps doing its own correlation; this skill answers the cross-customer questions from the agent you already use.

## The pain this closes

Firebox Management, Endpoint Security, ThreatSync, NDR and license allocation are separate WatchGuard APIs, each with its own paths, paging and status codes. Firebox and ThreatSync can answer across customers with a hierarchy or tenants flag, but WatchGuard Endpoint Security answers for a managed customer only with an access token minted for that customer: you look up the account's **audience** value and request a token with it (WatchGuard API docs, **Authentication**). WatchGuard's own blog celebrated Service Provider-level firmware upgrades because MSPs "no longer need to go down the line on each subscriber account, one at a time" (September 2022). And allocations drift from reality: a [WatchGuard Community thread](https://community.watchguard.com/watchguard-community/discussion/3267/removing-retired-and-unallocated-firebox-from-watchguard-cloud) (May 2023, still asked about in September 2024) shows retired, unallocated Fireboxes inflating the inventory with a support case as the only way out.

- `watchguard-cli accounts health` - one ranked row per customer: offline Fireboxes, at-risk endpoints, open incidents by severity, expiring licenses.
- `watchguard-cli endpoint-devices posture` - every unprotected, outdated, isolated, unlicensed or reboot-pending computer, across every customer.
- `watchguard-cli threatsync-incidents queue` - open incidents ranked by severity, threat score and age, with untouched ones and recommended-but-not-taken actions called out.
- `watchguard-cli allocations reconcile` - allocated serials with no reporting Firebox, reporting Fireboxes with no allocation, and what expires soon.
- `watchguard-cli firebox-transactions wait` - a pass or fail exit code for a deployment instead of refreshing a status page.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The WatchGuard MCP server runs locally, but it speaks HTTP natively: start it with `WATCHGUARD_MCP_HTTP_TOKEN=<a-bearer-token> watchguard-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default and refuses to start without that token; callers send it as `Authorization: Bearer`) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your WatchGuard credentials once.

### Is my WatchGuard data safe?

The CLI, MCP server and SQLite mirror run on **your machine**, under your user account, and MSP Skills never bundles or transmits your WatchGuard credentials. What your agent reads does go to your AI provider: query results, and the secrets that credential reads return (BOVPN pre-shared keys, private keys, Firebox passwords, activation and license keys). Keep those commands human-in-the-loop as the safety tiers below describe.

### What credentials do I need?

WatchGuard Cloud API access: as an Owner or Administrator, open **Administration > Managed Access** and click **Enable API Access** (a Service Provider does this from Subscriber view of its own account). You get a read-write Access ID, a read-only Access ID, the passwords you set for each, and an API key. Set `WATCHGUARD_CLIENT_ID` to an Access ID, `WATCHGUARD_CLIENT_SECRET` to its password, `WATCHGUARD_API_KEY` to the API key and `WATCHGUARD_ACCOUNT_ID` to your account ID, then run `watchguard-cli doctor`. For a reporting-only agent, use the read-only Access ID.

### Does it work across all my customers, or one account at a time?

Across all of them, with a Service Provider account. `accounts health`, `endpoint-devices posture` and `endpoint-patches exposure` list the accounts you manage, mint a separate token for each one and report every customer in one result, naming any account they could not reach. `threatsync-incidents queue` reads incidents for all your managed accounts in one ThreatSync call. If any read fails, including a product a customer does not have (WatchGuard answers that like any other refusal), these commands exit 6 (5 when nothing could be read) and say which account and product; `accounts health` lists that account first with `complete: false` instead of showing zeros, and an account it could not reach at all appears in `fetch_failures`. Add `--allow-partial-failure` to exit 0 on a partial result; it does not override exit 5. To work on one customer, set `WATCHGUARD_AUDIENCE` to that customer's audience value (`watchguard-cli audiences --account-id <customer-account-id>` returns it) and `WATCHGUARD_ACCOUNT_ID` to that customer's account ID. That is a convenience, not a security boundary: WatchGuard's audience token also works on your own account.

### My WatchGuard Cloud account is in Europe or Asia Pacific. Does it work?

Yes. Set `WATCHGUARD_BASE_URL` and `WATCHGUARD_TOKEN_URL` to your region's gateway: `https://api.deu.cloud.watchguard.com` for EMEA or `https://api.jpn.cloud.watchguard.com` for APAC, with `/oauth/token` on the end for the token URL. The default is the North America gateway, `api.usa.cloud.watchguard.com`. **Managed Access** in WatchGuard Cloud shows your account's API URL as `https://api.<region>.cloud.watchguard.com/rest/`; drop the `/rest/` when you set `WATCHGUARD_BASE_URL` (the CLI adds it to every path).

### Will this hit my WatchGuard API rate limits?

WatchGuard allows 500 requests per second and 200,000 requests per day per API key; a 429 means the day's quota is spent until 00:00 UTC. `accounts health`, `endpoint-devices posture` and `endpoint-patches exposure` fetch at most four accounts in parallel by default (`--concurrency`), the cross-customer commands stop at `--max-pages`, `--rate-limit` caps requests per second, GET responses are cached for 5 minutes, and `sync` keeps a local copy that `search` and `analytics` read without calling the API.

### Can my AI isolate devices or change firewall policy?

Only if you let it. Write commands such as `endpoint-devices isolate`, `endpoint-devices uninstall`, `firebox-deployments` and the `delete` commands send as soon as they run; there is no confirmation prompt. Preview any of them with `--dry-run` first; through the MCP server there is no preview, `watchguard_execute` sends a write as soon as it is called. For a reporting agent, use the read-only Access ID, or set `WATCHGUARD_MCP_READ_ONLY=1` so the MCP server drops its write tools and refuses every non-GET call through `watchguard_execute`.

### Has it been tested against a live WatchGuard Cloud account?

Not yet. Every command was built from WatchGuard's published OpenAPI specifications and tested against a local mock of those APIs, not a real account. If you run it against your own WatchGuard Cloud account, tell us what worked or broke on the [issue tracker](https://github.com/Servosity/msp-skills/issues) or at a Build Session; a real MSP run is what earns the live-verified badge.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `accounts health`, `endpoint-devices posture`, `threatsync-incidents queue`, `endpoint-patches exposure`, `allocations reconcile`, `firebox-devices changes` / `firmware-lag`, `firebox-transactions wait`, `sync`, `search`, `analytics`, every list / get command except the credential reads in the last row | Allow |
| Endpoint and incident actions | `endpoint-devices isolate` / `stop-isolation` / `scan` / `action` / `uninstall`, `endpoint-configurations assign`, `threatsync-actions perform`, `threatsync-incident-actions perform`, `threatsync-transactions`, `threatsync-incidents set-status`, `ndr-smart-alerts close`, `authpoint push` | Human-in-the-loop, explicit confirmation |
| Firewall config and deployment | `firebox-deployments`, `firebox-policies create` / `update` / `set-enabled`, the Firebox and FireCloud exception creates and updates, `firebox-exceptions enable-global` / `disable-global`, `firebox-templates subscribe`, `firebox-bovpn-p1-settings create` / `update`, `firebox-transactions update`, `endpoint-risks update-config`, `ndr-assets create` / `update`, `threatsync-comments create` / `update`, `allocations allocate` / `update-hardware` / `update-software`, `accounts create` / `update` | Preview with `--dry-run`, then a human-approved write |
| Destructive | `accounts delete` (it targets `--account-id`, which defaults to your own `WATCHGUARD_ACCOUNT_ID`), `operators delete`, `allocations deallocate`, `firebox-transactions cancel`, `ndr-assets delete`, `firebox-policies delete`, `threatsync-comments delete`, and every other `delete` command | Human-in-the-loop only, explicit confirmation |
| Credential and admin | `firebox-bovpn-tunnels` list / get and writes (pre-shared keys), `firebox-certificates` list / get / create / install (private keys and passwords), `firebox-networks`, `firebox-users` and `firebox-content-filtering` list / get (PPPoE, Dynamic DNS, Wi-Fi and override passwords), `activations list-recent` / `status` / `activate` (activation keys), `allocations list-owned` / `list-assigned` / `summary` (serial numbers and license keys), `export activations` (activation keys), `import`, `endpoint-installers` (enrollment download links), `operators create` / `update`, `audiences`, the `authpoint` password, OTP, QR code, forgot-token, check-policy and transaction-status calls, `auth login`, `auth set-token` | Human-in-the-loop only |

The local mirror and the response cache never store the secret fields those credential reads return, and allocation rows reach the local mirror with every software license key in `serialOrLicense` replaced by a one-way fingerprint (`license:` plus 16 hex characters); Firebox and access point serial numbers are kept, the row's own `productName` decides which; see [governance.md](./governance.md). Through the MCP server these reads return their secrets to the agent like any other result.

The strongest control is **which Access ID you give it**: with the read-only Access ID, WatchGuard refuses every write. Set `WATCHGUARD_MCP_READ_ONLY=1` to run the MCP server read-only. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Built from WatchGuard's published API specifications and tested against a local mock; no MSP has run it against a live WatchGuard Cloud account yet. Bring it to a weekly Build Session to be the first: [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-15._
