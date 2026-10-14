# Google Workspace + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Google Workspace
> API. Not affiliated with, endorsed by, or sponsored by Google LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/google-workspace/">
    <img src="../../docs/assets/video/google-workspace/animated-og.gif" alt="Google Workspace demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/google-workspace/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Cross-client Google Workspace admin: 2SV gaps, license waste, sharing drift, safe offboarding. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Google Workspace skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `google-workspace-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `google-workspace-mcp` over HTTPS behind an OAuth-capable gateway, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `google-workspace-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `google-workspace-mcp --transport http` and expose that behind HTTPS through an OAuth-capable gateway (ChatGPT cannot send the server's bearer token itself). See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `google-workspace-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Google Workspace MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/google-workspace-v0.1.0/google-workspace-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Google Workspace release on the [releases page](https://github.com/servosity/msp-skills/releases?q=google-workspace).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install google-workspace@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Google Workspace Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex`. Then authenticate per the README and run `google-workspace-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh)
```

The installer drops both `google-workspace-cli` (the CLI) and `google-workspace-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
google-workspace-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update google-workspace@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/google-workspace --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/google-workspace --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `google-workspace-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the google-workspace skill from https://github.com/servosity/msp-skills/tree/main/skills/google-workspace. The skill defines how its required CLI (`google-workspace-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Each client is a **tenant**: a Google Cloud service account with domain-wide delegation that impersonates that client's super admin. Set one up per client:

1. In a Google Cloud project, enable the **Admin SDK API** and the **Enterprise License Manager API** (plus the **Cloud Identity API** if you will use the `policy` profile, and the **Google Workspace Reseller API** for your reseller domain), create a service account, and download its JSON key. (Google Cloud organizations created since May 2024 block key creation by default; an org admin has to allow it for that project.)
2. Register the client. The key is copied into the CLI's config directory with owner-only permissions (mode 0600 on macOS and Linux; on Windows the file inherits the config folder's permissions), so delete the downloaded original afterwards:

   ```bash
   google-workspace-cli tenants add acme --domain example.com --admin admin@example.com --key ./acme-sa.json
   ```

3. Paste the printed Client ID and scopes into the client's **Admin console > Security > Access and data control > API controls > Domain-wide delegation** (`google-workspace-cli tenants scopes acme` prints them again).
4. Confirm Google issues a token for every scope you delegated:

   ```bash
   google-workspace-cli tenants test acme
   ```

The default `read` scope profile covers every report. Profiles combine with commas, and only `full` includes `read`: use `--scopes full` for offboarding writes (it also authorizes every user write, including deleting users and granting super admin), `--scopes read,policy` (or `full,policy`) to add the current 2SV, Drive sharing, and Gmail forwarding policy to `posture`, and register your own reseller domain with `--reseller --scopes reseller` (it is excluded from `--all-tenants`). `--scopes policy` on its own replaces `read` and leaves the reports nothing to read. Pick a client per command with `--tenant <name>` (or `GOOGLE_WORKSPACE_TENANT`), or fan out with `--all-tenants`.

Managing a single organization? Set `GOOGLE_WORKSPACE_TOKEN` to an OAuth access token that carries the Admin SDK scopes instead of registering tenants; the reports then run against that one organization. Do not leave it set once tenants are registered: any API command run without `--tenant` falls back to that token and acts on its organization, not a client's.

`google-workspace-cli tenants test --all-tenants` is the real credential check: it mints a token per scope for every registered client, reads each client's customer record and domains, and exits non-zero when any delegation is missing. It cannot tell you whether the License Manager or Cloud Identity APIs are enabled (the first report that needs them will), and with no tenants registered it has nothing to check. `doctor` probes one real read endpoint with the selected tenant's token (`--tenant <name>`, or the only registered tenant) or the single-org token and reports a rejected or wrong-host credential as `ERROR`, but it exits 0 unless you pass `--fail-on error`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which accounts across all my clients lack 2-Step Verification, admins first? | `google-workspace-cli mfa-gaps --all-tenants` |
| Which Google Workspace licenses sit on suspended, never-signed-in, or idle accounts? | `google-workspace-cli stale-licenses --all-tenants` |
| Did our suspended leavers keep a license, groups, an admin role, OAuth tokens, or an unfinished Drive transfer? | `google-workspace-cli leavers --all-tenants` |
| One row per client for the QBR: 2SV coverage, admins without 2SV, stale licenses, Drive and Calendar setting changes | `google-workspace-cli posture --all-tenants` |
| Who changed Drive or Calendar settings, and which files went external or public this week? | `google-workspace-cli sharing-drift --all-tenants --since 7d` |
| Which third-party apps hold full mail, full Drive, or admin access to client data? | `google-workspace-cli oauth-apps --all-tenants --min-risk high` |
| Any suspicious sign-ins, hijacked accounts, or 2SV turned off this week? | `google-workspace-cli login-alerts --all-tenants --since 7d` |
| What would offboarding this leaver do, step by step, before anything changes? | `google-workspace-cli offboard "jane@example.com" --successor "manager@example.com" --ou "/Departed"` |
| Is every client's domain-wide delegation still authorized? | `google-workspace-cli tenants test --all-tenants` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Google Workspace MCP servers, such as [taylorwilsdon/google_workspace_mcp](https://github.com/taylorwilsdon/google_workspace_mcp), work inside one organization's Gmail, Drive, and Docs. [GAM](https://github.com/GAM-team/GAM), the tool most MSPs use for Workspace administration, keeps one config section per client and can loop a command over every section into one CSV, but you write the loop, the query, and any join across users, licenses, admin roles, and audit logs yourself, for each question.

This skill keeps a **registry of client tenants**, each with its own service account and least-privilege scope profile, and fans one question out across all of them in parallel. `mfa-gaps`, `stale-licenses`, `leavers`, `posture`, `sharing-drift`, `oauth-apps`, and `login-alerts` already do those joins per client and return one ranked table; a client it could not read is listed in `fetch_failures`, never silently dropped. For deeper single-client work, `sync` pulls one tenant into a **local SQLite mirror** (kept separate per tenant) for `search`, `analytics`, and the MCP server's `sql` tool.

## The pain this closes

On r/msp's ["Google Workspace Management"](https://www.reddit.com/r/msp/comments/1tu2nas/google_workspace_management/) thread (June 2026), the original poster asked for "a CIPP equivalent for the GW side." An MSP managing about 10 Workspace tenants out of roughly 250 clients called it "a manual process for the most part," and the most common answer was multi-tenant GAM: "you just GAM select company to switch companies." Microsoft gives MSPs GDAP and CIPP; Google gives them a separate Admin console per client. So the questions an owner asks across the whole book - who lacks 2SV, where licenses are wasted, which leavers were never fully cleaned up - become a loop of logins, exports, and spreadsheet joins. Leavers cost real money: Google bills a suspended account the same as an active one.

This skill closes that gap:

- **`google-workspace-cli mfa-gaps --all-tenants`** - every account without 2SV across every client, unenrolled admins first.
- **`google-workspace-cli stale-licenses --all-tenants`** - Workspace edition seats on suspended, never-signed-in, or idle accounts, ready to reclaim (`--products` replaces the default `Google-Apps`, so list it too: `--products Google-Apps,Google-Vault`).
- **`google-workspace-cli leavers --all-tenants`** - suspended accounts still holding a license, groups, an admin role, OAuth tokens, or an unfinished Drive transfer.
- **`google-workspace-cli offboard "jane@example.com" --successor "manager@example.com"`** - the offboarding runbook, previewed with live reads before anything changes, with suspend last because backup-code cleanup fails on a suspended account.
- **`google-workspace-cli posture --all-tenants`** - one row per client for the QBR.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Google Workspace MCP server runs locally, but it speaks HTTP natively: start it with `google-workspace-mcp --transport http` (it listens on 127.0.0.1:7777 by default) and put its /mcp endpoint behind HTTPS. ChatGPT connectors sign in with OAuth (or no authentication) and cannot send the fixed bearer token the server requires, so that HTTPS endpoint has to be an OAuth-capable gateway that adds the token: a build-and-host task, not a self-serve install. Details in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You register each client once with `google-workspace-cli tenants add` (see Authenticate above).

### Is my Google Workspace data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Each client's service-account key is copied into the CLI's config directory with owner-only permissions (mode 0600 on macOS and Linux; on Windows the file inherits the config folder's permissions) and used only to mint short-lived tokens from Google; keys are never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Google partner or reseller?

No. Each client tenant is just a service account that the client's own super admin authorizes for domain-wide delegation. Reseller status only matters for the Reseller API commands (`reseller-customers`, `subscriptions`), which run against your own reseller domain registered with `--reseller --scopes reseller`.

### Can an agent change my clients' Google Workspace, or only read?

Only what you authorize per client. The default `read` scope profile covers every report, but Google has no read-only scope for OAuth tokens and app passwords, and none for license assignments, so that profile can still revoke tokens, sign users out, read backup codes, turn off 2SV, and change licenses. Keep those commands human-in-the-loop (see the safety model below). Offboarding writes need `--scopes full`, and `offboard` only previews unless you type `--yes` yourself, which an MCP client cannot pass.

### Will this hit Google's API quotas?

The cross-client commands query Google live, so they count against the Admin SDK quota of the Google Cloud project that owns each client's service account. `--concurrency` caps how many clients run in parallel, `--max-scan-pages` caps paging, and a client whose calls fail or hit the cap is listed in `fetch_failures` instead of silently dropped.

### Does it replace the Google Admin console?

No. The Admin console stays the place for deep single-client work and for settings the API does not expose. This skill adds the cross-client view the console does not have and lets your AI agent drive it in plain English.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `mfa-gaps`, `stale-licenses`, `leavers`, `posture`, `sharing-drift`, `oauth-apps`, `login-alerts`, `tenants list`, `tenants test`, `users list`, `sync`, `offboard "<email>"` without `--yes` (a preview) | Allow |
| **Write (routine)** | `users insert`/`patch`, `users aliases insert`, `groups insert`/`patch`, `members insert`, `orgunits insert`/`patch`, `tenants add` (local config only) | Preview with `--dry-run`, then a reviewed write |
| **Credential / security** | `verification-codes list` (returns a user's live backup codes), `verification-codes generate`/`invalidate`, `two-step-verification` (turns 2SV off), `tokens delete`, `asps delete`, `users sign-out`, `users make-admin`, `role-assignments insert`, `roles insert`/`patch`/`update` | Human-in-the-loop only |
| **Data egress** | `activities watch`, `users watch`, `users aliases watch` (Google pushes a client's audit events or user changes to the `--address` URL you give it; they work under the default `read` profile) | Human-in-the-loop only |
| **Destructive / billing** | `offboard --yes`, `users delete`, `groups delete`, `domains delete`, `transfers insert`, `license-assignments insert`/`update`/`delete`, `subscriptions insert`/`change-plan`/`change-seats`/`change-renewal-settings`/`start-paid-service`/`activate`/`suspend`/`delete`, `reseller-customers insert`, `mobiledevices action` (device wipe), `chromeosdevices action`, `customer devices-chromeos issue-command`/`batch-change-status` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope profile you authorize in each client's Admin console** - the CLI can only do what that client's domain-wide delegation permits. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Google Workspace API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-13._
