---
layout: default
title: "Hudu MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Hudu cmdlet, plus an offline SQLite mirror, cross-entity audits, and agent-native output no PowerShell module or read-only MCP ships."
permalink: /skills/hudu/
skill_name: "Hudu MCP"
image: /assets/social/hudu/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Hudu MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Hudu data safe?"
    a: "Your data stays on your machine - the CLI, MCP server, and the local mirror are all local. The password audit never reads or stores secret values; it uses only each entry's name, username, and last-updated date. Credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will a company-scoped Hudu API key work?"
    a: "Mostly. Audits and search run over whatever you've synced, so they work with any key. The one limit: Hudu's global asset-list endpoint requires a global (not company-scoped) key - with a scoped key, use 'assets list-by-company <company_id>' instead. The 'onboard --apply' write path also needs a global key."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Hudu credentials once; hudu-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Hudu question in plain language; it runs hudu-cli for you."
---

# Hudu + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Hudu
> API. Not affiliated with, endorsed by, or sponsored by Hudu Technologies, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask in plain English which clients have the worst documentation, which vault passwords are overdue for rotation, and what SSL certs, domains, or warranties expire next - across every company at once. Hudu plus your AI agent reads a local mirror of your whole instance, so the hygiene questions the portal makes you click through company by company become one instant, reproducible answer.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/hudu){: .btn}

## Instead of clicking through Hudu, just ask

**Instead of** Open each client's asset layouts in the Hudu portal and eyeball which assets are missing required fields
**just ask:** *"Which clients have the worst documentation completeness right now?"*
<sub>Your agent runs: <code>hudu-cli audit completeness --agent</code></sub>

**Instead of** Click into every company's password vault to find credentials nobody has rotated in months
**just ask:** *"Show me vault passwords that haven't been rotated in 180 days, grouped by company"*
<sub>Your agent runs: <code>hudu-cli audit stale-passwords --older-than 180d --agent</code></sub>

**Instead of** Reconcile SSL, domain, and warranty expiry dates across tenants in a spreadsheet before they lapse
**just ask:** *"What expires in the next 30 days across all my clients?"*
<sub>Your agent runs: <code>hudu-cli audit expirations --within 30d --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/hudu/wide-1200x630.png" src="/assets/video/hudu/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/hudu/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which clients have the worst documentation completeness? | `hudu-cli audit completeness --agent` |
| What's expiring in the next 30 days across every client? | `hudu-cli audit expirations --within 30d --agent` |
| Which vault passwords are overdue for rotation? | `hudu-cli audit stale-passwords --older-than 180d --agent` |
| Which knowledge-base articles are stale and probably out of date? | `hudu-cli audit stale-articles --older-than 365d --agent` |
| Which assets have drifted from their layout's current schema? | `hudu-cli audit layout-drift --agent` |
| Give me one worst-first hygiene scorecard across every company. | `hudu-cli audit summary --agent` |
| Find everything matching a keyword across all synced docs. | `hudu-cli search "vpn gateway" --agent` |
| Resolve this Hudu link to its asset, company, and relations. | `hudu-cli resolve "https://docs.example.huducloud.com/a/dc01-abc123" --agent` |
| Which PSA/RMM records don't map to a live Hudu asset? | `hudu-cli reconcile --agent` |
| Scaffold a new client's docs from our house template. | `hudu-cli onboard --company 42 --template msp-standard` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/hudu/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/hudu/guide.md).

## What makes this one different

Most Hudu integrations and MCP servers proxy each question into a live API call - fine for one lookup, useless when you ask about every client at once. This skill syncs your whole Hudu instance into a local SQLite mirror, so cross-tenant hygiene questions become one offline query: instant, fully paginated, and reproducible later as a snapshot you can diff against.

The Hudu portal answers one company on one page at a time; this skill answers across every client at once and hands the result to the AI agent you already work in. It complements the web app you still use for editing and day-to-day documentation - it doesn't replace it.

## The pain this closes

- Documentation rots silently: assets get created with half their required fields blank, and nobody notices until a tech needs the missing detail mid-incident.
- Hudu has no native password-expiration tracking, so stale vault credentials pile up unseen across dozens of client vaults.
- Expiring SSL certificates, domains, and warranties hide one company at a time in the portal - you find the lapse when the client's site goes down, not before.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hudu/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hudu/install.ps1 | iex
```

After install, authenticate once with your Hudu credentials, then verify with `hudu-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | audit completeness, audit stale-passwords, audit expirations, audit summary, search, resolve, reconcile, companies list, assets list-by-company, sync, doctor | Allow |
| Write (routine) | assets create, assets update, articles create, articles update, companies create, companies update, asset-layouts create, onboard --apply | Preview with --dry-run, then a reviewed write |
| Destructive / credential | assets delete, articles delete, companies delete, asset-passwords delete, asset-passwords get, asset-passwords list, auth set-token, auth logout | Human-in-the-loop only |

The skill reads everything over a local mirror - audits, search, resolve, and reconcile are always safe to run and cannot change anything. Writes (creating or updating assets, articles, companies, and asset layouts, plus 'onboard --apply') send to your live Hudu instance, so the recommended agent policy is preview-then-approve. Reading or changing password-vault entries and managing the API credential are credential-tier; deletes and archives are destructive - keep both human-in-the-loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/hudu/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Hudu MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Hudu data safe?

Your data stays on your machine - the CLI, MCP server, and the local mirror are all local. The password audit never reads or stores secret values; it uses only each entry's name, username, and last-updated date. Credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will a company-scoped Hudu API key work?

Mostly. Audits and search run over whatever you've synced, so they work with any key. The one limit: Hudu's global asset-list endpoint requires a global (not company-scoped) key - with a scoped key, use 'assets list-by-company <company_id>' instead. The 'onboard --apply' write path also needs a global key.


## Status

Beta. Validated against the Hudu API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
