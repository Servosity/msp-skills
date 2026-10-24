# CIPP + AI: MCP server for Microsoft 365, Entra ID, Intune, Exchange, Defender and GDAP - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the CIPP
> API. Not affiliated with, endorsed by, or sponsored by CyberDrain.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/cipp/">
    <img src="../../docs/assets/video/cipp/animated-og.gif" alt="CIPP + Microsoft 365 demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/cipp/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

MCP server and CLI for CIPP - Microsoft 365, Entra ID, Intune, Exchange Online, Defender and GDAP across every client tenant, with a local store, offline joins across endpoints, and history over time. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

> The write preview, read-only switch, token-protected HTTP mode and local-data controls described on this page ship in cipp 0.1.13 and later. Check with `cipp-cli --version`.

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the CIPP skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `cipp-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `cipp-mcp` over HTTPS with `CIPP_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `cipp-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `cipp-mcp --transport http` with `CIPP_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `cipp-mcp` over HTTPS with `CIPP_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `cipp-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the Path A or Path B installer below first (or set `CIPP_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

[**Download CIPP MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/cipp-v0.1.12/cipp-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every CIPP release on the [releases page](https://github.com/servosity/msp-skills/releases?q=cipp).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install cipp@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the CIPP Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex`. Then authenticate per the README and run `cipp-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)
```

The installer drops both `cipp-cli` (the CLI) and `cipp-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
cipp-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update cipp@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/cipp --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/cipp --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cipp-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cipp skill from https://github.com/servosity/msp-skills/tree/main/skills/cipp. The skill defines how its required CLI (`cipp-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

CIPP is self-hosted, so the CLI needs your instance's API base URL (the value CIPP shows you, ending in `/api`) and a credential from an API client you create in CIPP under **Integrations > CIPP-API**. Let the CLI run the OAuth2 client-credentials exchange, reading the client secret from stdin so it stays out of your shell history:

```bash
cipp-cli auth login --client-id 00000000-0000-0000-0000-000000000000 --tenant-id 00000000-0000-0000-0000-000000000001 --base-url https://cipp.yourmsp.com/api --client-secret - < secret.txt
```

Or set a bearer token in the environment.

**macOS / Linux:**

```bash
CIPP_API_KEY=<value> CIPP_BASE_URL=https://cipp.yourmsp.com/api cipp-cli doctor
```

**Windows (PowerShell):**

```powershell
$env:CIPP_API_KEY = "<value>"; $env:CIPP_BASE_URL = "https://cipp.yourmsp.com/api"; cipp-cli doctor
```

`doctor` calls a real CIPP endpoint with your credential and prints where the CLI keeps its files. It exits 0 even when the credential is rejected, so scripts should add `--fail-on error`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which tenants have users with no MFA registered, as one row per tenant? | `cipp-cli posture --dimension mfa` |
| How does Conditional Access compare across all tenants? | `cipp-cli posture --dimension ca` |
| Which tenants lack a Conditional Access policy most of the fleet has? | `cipp-cli ca diff` |
| Which delegated-admin (GDAP) relationships expire in the next 30 days? | `cipp-cli gdap expiring --days 30` |
| What changed in admin roles since last week? | `cipp-cli changes --resource roles --since 7d` |
| Everything stored about one client tenant, in one record | `cipp-cli scorecard --tenant contoso.onmicrosoft.com` |
| Pull one read across every client tenant and keep it locally | `cipp-cli fanout --endpoint /ListMFAUsers --all-tenants --save` |
| Plan a batch of offboards from a CSV before anything runs | `cipp-cli bulk --from offboards.csv` |
| Delete the local store, response cache and learn journal | `cipp-cli local-data purge --yes` |
| Are my CIPP credentials and connectivity healthy? | `cipp-cli doctor` |

The rollups (`posture`, `ca diff`, `changes`, `scorecard`) read the local store, so run `fanout --save` for the endpoints they need first; each command's `--help` names them. `fanout --save` refuses a tenant's response when anything in the response reads as a failure: failure wording or a failed state anywhere under a status-like field (state, status, result, message, text, detail, reason and the like, at any depth: `"status":{"code":"error"}`) or in a bare message, a populated field named like error, exception or fault (`"error":"HTTP 403"`, whatever the wording), a field named like success or ok that is `false`, or a failed state, in the rows or beside them, and keeps the rows already stored. It stores only a list whose every entry carries the endpoint's id; JSON `null`, an empty answer, a single object or a message is shown, not stored. In the tenant list, and only there, tenant records keep their own Graph token fields (`LastGraphError`, `GraphErrorCount`), so one tenant with a failed Graph token does not stop the fleet. A response with a duplicate key is refused. A row whose own name holds a failure word (a user named "Error Reporting", a group named "Denied Access") is still a row. The trade-off: a list whose rows report their own failed status is refused too; run the same `fanout` with `--json` to see what CIPP returned. Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

CIPP already answers estate-wide questions itself. Its All Tenants views cover the MFA report, Standards alignment and the licence report, its API returns one estate-wide response for `tenantFilter=AllTenants`, and it ships its own MCP server. If one of those answers your question, use it: it is one call, served from CIPP's reporting cache.

This connector is a different shape: it is **local-first**. `fanout` runs a CIPP read once per tenant and, with `--save`, keeps the rows in a SQLite file on your machine, each save a dated snapshot. Your agent can then search, join and compare that copy offline - a per-tenant MFA or Conditional Access table in `posture`, every stored signal for one tenant in `scorecard`, the policy one tenant lacks in `ca diff`, your own snapshots over time in `changes` and `standards drift` - without another API call. You already have this data through CIPP; the connector puts a copy where an agent can work across it. It also carries CIPP's full command surface, writes included: in the MCP server, a CIPP API call that changes something comes back as a preview and runs only when the agent confirms it.

The cost of that shape is real: one call per tenant instead of one call for the estate, and a copy of tenant data on your disk. The next section says where it lives and how to remove it.

## Where your data lives

The local copy is on purpose (it is what an agent searches and joins), and it is **not encrypted**. New files are created readable by your account only; beyond that it relies on your operating system's account permissions and full-disk encryption (BitLocker, FileVault).

- **Local store:** a `data*.db` SQLite file in the CLI's data directory, filled by `fanout --save` and by reads in the default `--data-source auto` mode.
- **Response cache:** GET responses for five minutes under `http/` in the cache directory. Expired files are deleted on a later run.
- **Credentials:** `credentials.toml` in the data directory. After `auth login` it holds the API client's secret (so the CLI can mint new tokens) and the current token. With `CIPP_NO_CONFIG_WRITE=1`, `auth login` refuses and nothing is stored; set `CIPP_API_KEY` instead.
- **Learn journal:** a record of the commands you ran, under `learn/` in the state directory. `CIPP_NO_LEARN=1` turns it off.
- **Resume checkpoints:** `fanout --resume` keeps the tenant domains it has finished in the state directory.
- `cipp-cli doctor` prints each path for your machine.

Before a successful response is shown, cached or saved, it is checked for your configured credentials (the API key, the client secret, the minted token) in every encoding the CLI can decode, with spaces and invisible characters removed first. A response holding a single value over 64 KiB once spaces are removed, or a response that is not JSON and is over 64 KiB, is withheld instead, because it cannot be decoded whole; read that value in CIPP itself.

To delete the store, cache and journal, run `cipp-cli local-data purge` (lists what it would delete) and then `cipp-cli local-data purge --yes`; `cipp-cli auth logout` removes the stored credentials. To keep no local copy of CIPP data, set `CIPP_NO_LOCAL_DATA=1`: reads go live, the response cache is off, and `fanout --save` and `fanout --resume` refuse. In Claude Desktop it is the extension's **Run without a local copy** setting; for other MCP hosts, put it in the server's `env` block.

When a technician leaves, removing their access in CIPP does not touch what is already on their machine: run `cipp-cli auth logout` and `cipp-cli local-data purge --yes` there, delete the `checkpoints` folder in the CLI's state directory, and rotate the API client's secret in CIPP.

## The pain this closes

CIPP's own views answer questions about the current state. An AI agent working from live answers alone starts over on every question, and comparing this quarter with last needs dated copies you keep yourself. Scripting CIPP has its own costs: each API client is limited to 100 requests per 10 seconds, per the [CIPP API documentation](https://docs.cipp.app/api-documentation/setup-and-authentication), and some write endpoints answer HTTP 200 with the failure written inside the response's `Results`, so a batch has to read every response to know what happened.

- **`cipp-cli fanout --endpoint /ListMFAUsers --all-tenants --save`** - one read across every tenant, saved locally as a dated snapshot, with HTTP 429 Retry-After handling and `--resume` after a halt.
- **`cipp-cli posture --dimension mfa`** (also `ca`, `standards`, `dlp`, `retention`, `labels`) - one tenant-by-metric table from the saved rows.
- **`cipp-cli ca diff`** - the Conditional Access policy one tenant lacks that most of the fleet has.
- **`cipp-cli changes --resource roles --since 7d`** - rows added, removed or changed between two of your own snapshots.
- **`cipp-cli bulk --from offboards.csv --execute`** - a CSV-driven batch of user and mailbox actions: plan-only without `--execute`, a non-zero exit when CIPP reports a failure inside an HTTP 200, and per-row `--resume`.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The CIPP MCP server runs locally over stdio by default. Its HTTP mode (`cipp-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `CIPP_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your CIPP credentials once.

### Is my CIPP data safe?

The CLI and MCP server run on your machine and talk only to your CIPP instance; MSP Skills receives nothing. But the connector does keep an unencrypted local copy of what it reads, so whoever can read your user profile can read that copy. [Where your data lives](#where-your-data-lives) lists the files, how to delete them, and how to run with no local copy (`CIPP_NO_LOCAL_DATA=1`).

### Doesn't CIPP already show this across all tenants?

Often, yes. CIPP's All Tenants views show the MFA report, Standards alignment and the licence report across your estate, and its own MCP server can query `AllTenants`. This connector does not replace those. It keeps a local, dated copy so an agent can join endpoints and compare your own snapshots offline, and it gives the agent CIPP's write endpoints behind a preview-then-confirm step.

### Do I need my own CIPP instance to use this?

Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via `cipp-cli auth login`, or you pipe a bearer token to `cipp-cli auth set-token`. It reads and acts through your CIPP - it does not replace it.

### Will this hit CIPP's API rate limit?

It can: `fanout` makes one call per tenant, and CIPP allows 100 requests per 10 seconds per API client. The client waits out HTTP 429 using Retry-After, `--concurrency` sets how many tenants run at once, and `--resume` skips tenants a halted run already finished. Once rows are saved, the rollups read the local store and make no API calls.

### Can this change my tenants, or only read?

Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. In the MCP server, any CIPP API call that changes something returns a preview first and runs only when the agent confirms it; set `CIPP_MCP_READ_ONLY=1` and the server drops its write tools. On the command line, `bulk` plans by default and writes only with `--execute`, and other write commands preview with `--dry-run`. The strongest boundary is still the role you give the API client in CIPP.

### Does it replace the CIPP portal?

No. CIPP stays your portal. This connector lets your AI agent work across a local copy of what CIPP returns and drive CIPP from natural language.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read (rollups and lists)** | `cipp-cli posture --dimension mfa`, `cipp-cli ca diff`, `cipp-cli gdap expiring --days 30`, `cipp-cli fanout --endpoint /ListUsers --all-tenants`, `cipp-cli list-tenants`, `cipp-cli doctor` | Allow |
| **Write (routine)** | `cipp-cli add-user`, `cipp-cli edit-user`, `cipp-cli bulk --from changes.csv --execute` (plans by default; writes only with `--execute`) | Preview with `--dry-run`, then a reviewed write |
| **Credential / security** | `cipp-cli exec-reset-mfa`, `cipp-cli exec-per-user-mfa`, `cipp-cli exec-get-local-admin-password`, `cipp-cli exec-token-exchange` | Human-in-the-loop only |
| **Destructive** | `cipp-cli exec-offboard-user`, `cipp-cli remove-user`, `cipp-cli exec-device-delete`, `cipp-cli delete-sharepoint-site` | Human-in-the-loop only, explicit confirmation |

Some CIPP endpoints are called with GET but change things (for example `RemoveStandard`, `ExecSendOrgMessage`, `ExecRunBackup`). The MCP server treats every one of them (43 in this build) as a change that needs confirmation, and `fanout`, `export` and `tail` refuse them. The typed write commands on the command line print CIPP's `Results` text and exit 0 even when that text reports a failure, so read it before reporting success; `bulk` and `cipp_execute` turn such a failure into an error. The strongest control is the **scope you grant the CIPP credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Live-verified: Abhi Saini, Bearium Networks (MSP), ran cross-tenant `fanout` against a live CIPP tenant on 2026-08-13 ([receipt](https://compoundingteams.com/)). More MSPs run it live in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-23._
