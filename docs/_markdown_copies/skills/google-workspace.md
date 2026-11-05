> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/google-workspace/

# The Google Workspace MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Google LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Google Workspace. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Google Workspace to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Google gives MSPs no cross-client admin console: every client is its own Admin console login. Register each client once (a service account with domain-wide delegation), then ask your AI "who still has no 2-Step Verification across all my clients," "which paid licenses can I reclaim," or "were our suspended leavers fully offboarded," and get one ranked answer across every tenant, read live and in parallel.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/google-workspace/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/google-workspace)

## Instead of clicking through Google Workspace, just ask

**Instead of** Logging into each client's Admin console, filtering Users by 2-Step Verification enrollment, and copying the unenrolled admins into a spreadsheet, one client at a time
**just ask:** *"Which accounts across all my clients still don't have 2-Step Verification, admins first?"*
Your agent runs: `google-workspace-cli mfa-gaps --all-tenants --agent`

**Instead of** Exporting each client's users and license assignments and matching suspended, never-signed-in, and idle accounts that still hold a paid seat
**just ask:** *"Which paid Google Workspace licenses can I reclaim across my clients?"*
Your agent runs: `google-workspace-cli stale-licenses --all-tenants --agent`

**Instead of** Working through the offboarding checklist by hand in the Admin console, where suspending the account too early blocks the backup-code cleanup that has to run first
**just ask:** *"Preview offboarding jane@example.com and hand her Drive and Calendar to her manager"*
Your agent runs: `google-workspace-cli offboard "jane@example.com" --successor "manager@example.com" --ou "/Departed"`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/google-workspace/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which accounts across all my clients lack 2-Step Verification, with unenrolled admins first? | `google-workspace-cli mfa-gaps --all-tenants --agent` |
| Which Google Workspace licenses sit on suspended, never-signed-in, or idle accounts, per client? | `google-workspace-cli stale-licenses --all-tenants --agent` |
| Did our suspended leavers keep a license, groups, an admin role, OAuth tokens, or an unfinished Drive transfer? | `google-workspace-cli leavers --all-tenants --agent` |
| Where does each client stand on 2SV coverage, admins without 2SV, stale licenses, and Drive, Calendar and app-sharing setting changes? | `google-workspace-cli posture --all-tenants --agent` |
| Who changed Drive or Calendar settings, and how many files went external or public (with sample names), in the last week? | `google-workspace-cli sharing-drift --all-tenants --since 7d --agent` |
| Which third-party apps hold full mail, full Drive, or admin access to client data? | `google-workspace-cli oauth-apps --all-tenants --min-risk high --agent` |
| Any suspicious sign-ins, hijacked accounts, or 2SV turned off across clients this week? | `google-workspace-cli login-alerts --all-tenants --since 7d --agent` |
| What would offboarding this leaver do, step by step, before anything changes? | `google-workspace-cli offboard "jane@example.com" --successor "manager@example.com" --ou "/Departed"` |
| Is every client's domain-wide delegation still authorized, scope by scope? | `google-workspace-cli tenants test --all-tenants --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/google-workspace/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/google-workspace/guide.md).

## What makes this one different

Most Google Workspace MCP servers, such as [taylorwilsdon/google_workspace_mcp](https://github.com/taylorwilsdon/google_workspace_mcp), work inside one organization's Gmail, Drive, and Docs. [GAM](https://github.com/GAM-team/GAM) keeps one config section per client and can loop a command over every section into one CSV, but you write the loop, the query, and any join across users, licenses, admin roles, and audit logs yourself. This skill keeps a registry of client tenants, each with its own service account and least-privilege scope profile, and fans one question out across all of them in parallel: 2SV gaps, license waste, incomplete offboarding, sharing drift, risky OAuth apps, and sign-in alerts come back as one ranked table, with any tenant it could not read listed as a failure rather than silently dropped.

The Google Admin console stays the place for deep single-client work and for settings the API does not expose. This skill adds what the console does not have: one view across every client domain you manage, driven from whichever AI agent you already use, plus an offboarding runbook that previews every step with live reads and suspends the account last, after the backup-code invalidation that fails on a suspended account.

## The pain this closes

- On r/msp's "Google Workspace Management" thread (June 2026) the original poster asked for "a CIPP equivalent for the GW side"; another MSP managing about 10 Workspace tenants out of roughly 250 clients called it "a manual process for the most part." Microsoft has GDAP and CIPP; Google has no cross-client admin view, so every client is a separate Admin console login.
- The common workaround is GAM: "multi tenant GAM ... you just GAM select company to switch companies," as one reply in the same thread put it. GAM can loop a command over every client section, but each fleet question still means writing that loop and query yourself, and anything that joins users with licenses, admin roles, or audit events is a script you maintain.
- Google bills a suspended account the same as an active one (Google Workspace Admin Help, "Preserve data for users who leave your organization"), so every leaver who was suspended instead of moved to an Archived User license, or never cleaned up at all, keeps costing the client a paid seat until someone finds it.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/google-workspace/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex
```

After install, register each client once with `google-workspace-cli tenants add` (a service account with domain-wide delegation, see the README's Authenticate section), then confirm with `google-workspace-cli tenants test --all-tenants`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | google-workspace-cli mfa-gaps --all-tenants; google-workspace-cli stale-licenses --all-tenants; google-workspace-cli leavers --all-tenants; google-workspace-cli posture --all-tenants; google-workspace-cli sharing-drift --all-tenants --since 7d; google-workspace-cli oauth-apps --all-tenants; google-workspace-cli login-alerts --all-tenants; google-workspace-cli users list; google-workspace-cli offboard "jane@example.com" (preview, no --yes); google-workspace-cli sync | Allow |
| Write (routine) | users insert/patch, users aliases insert, groups insert/patch, members insert, orgunits insert/patch, resources calendars insert, users undelete, domains insert, tenants add and tenants test (local config only; a tenant whose key resolves to another organization is quarantined and its local mirror moved aside) | Preview with --dry-run, then a reviewed write |
| Credential / security | verification-codes list (returns a user's live backup codes), verification-codes generate/invalidate, two-step-verification (turns 2SV off), tokens delete, asps delete, users sign-out, users make-admin, users patch/update with --password, role-assignments insert, roles insert/patch/update/delete | Human-in-the-loop only |
| Data egress | activities watch, users watch, users aliases watch: Google pushes a client's audit events or user changes to the --address URL you supply, and they work under the default read profile; also resellernotify register and any report run with --deliver webhook:<url> | Human-in-the-loop only |
| Destructive / billing | offboard --yes, users delete, groups delete, domains delete, transfers insert (moves Drive and Calendar ownership), license-assignments insert/update/patch/delete, subscriptions insert/change-plan/change-seats/change-renewal-settings/start-paid-service/activate/suspend/delete, reseller-customers insert, mobiledevices action (device wipe), chromeosdevices action, customer devices-chromeos issue-command/batch-change-status | Human-in-the-loop only, explicit confirmation |

Each client is a registered tenant whose service account impersonates that client's super admin with only the scopes you grant. The cross-client reports (mfa-gaps, stale-licenses, leavers, posture, sharing-drift, oauth-apps, login-alerts) only read. The default read scope profile still allows token revocation, sign-out, backup-code reads, 2SV turn-off, and license changes, because Google offers no read-only scope for those surfaces, so those commands are human-in-the-loop. Offboarding needs the full profile and only acts with an explicit --yes. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/google-workspace/governance.md).

## Frequently asked questions

### Is there an MCP server for Google Workspace?

Yes - this one. A free, open source MCP server and Claude Code Skill for Google Workspace, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Google Workspace MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `GOOGLE_WORKSPACE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `GOOGLE_WORKSPACE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way google-workspace-mcp runs, and shares MCP configuration with Codex: add google-workspace-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You register each client once with `tenants add`.

### Is my Google Workspace data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. Each client's service-account key is copied into the CLI's config directory with owner-only permissions (mode 0600 on macOS and Linux; on Windows the file inherits the config folder's permissions), and tokens are minted on demand and sent only to Google. Credentials are never bundled or transmitted by MSP Skills.

### How do I connect a client's Google Workspace?

In a Google Cloud project, create a service account and download its JSON key, and enable the Admin SDK API and the Enterprise License Manager API (plus the Cloud Identity API for the `policy` profile). Run `google-workspace-cli tenants add <name> --domain <client domain> --admin <client super admin> --key <key file>`. It prints the Client ID and the scopes to paste into the client's Admin console under Security, Access and data control, API controls, Domain-wide delegation. Then `google-workspace-cli tenants test <name>` confirms Google issues a token for every delegated scope.

### Does an agent get write access to my clients?

Only what you authorize per client. The default `read` scope profile covers the reports, but Google has no read-only scope for OAuth tokens or app passwords, and none for license assignments, so that profile can still revoke tokens, sign users out, read backup codes, turn off 2SV, and change licenses: keep those commands human-in-the-loop. Offboarding writes need the `full` profile, and `offboard` only previews unless you pass `--yes` on the command line, which an MCP client cannot do. Setting `GOOGLE_WORKSPACE_MCP_READ_ONLY` to exactly `1` also hides the MCP server's tools that change Google or your tenant registry and refuses every non-GET API call; tools that write only the local learn store stay.

### Will this hit Google's API quotas?

The cross-client commands query Google live, so they count against the Admin SDK quota of the Google Cloud project that owns each client's service account. `--concurrency` caps how many clients run in parallel, `--max-scan-pages` caps paging, and a client whose calls fail or hit the page cap is listed in `fetch_failures` rather than silently dropped.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Google Workspace API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
