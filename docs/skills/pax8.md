---
layout: default
title: "Pax8 MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces."
permalink: /skills/pax8/
skill_name: "Pax8 MCP"
image: /assets/social/pax8/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Pax8 MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Pax8 data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Pax8 partner, and what credentials does it use?"
    a: "Yes - it talks to the Pax8 Partner API with your own partner credentials. Create an OAuth2 Client ID and Client Secret in the Pax8 portal under Integrations, then set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET. The CLI exchanges them for a bearer token at Pax8's token endpoint and caches it. PAX8_AUDIENCE (default api://p8p.client) and PAX8_OAUTH_SCOPE are optional overrides. The credential's own permissions are the real boundary - scope it to what you want the AI to reach."
  - q: "Will this hit my Pax8 API rate limits?"
    a: "After the first sync, the analytics commands (reconcile, mrr, overage, spend, since, company show, search) run against your local SQLite mirror with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "Does this replace the Pax8 portal?"
    a: "No. Provisioning, ordering, and support workflows stay in the portal. This skill answers the cross-entity billing and revenue questions the portal cannot compose in one place, from your terminal or agent."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Pax8 credentials once; pax8-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Pax8 question in plain language; it runs pax8-cli for you."
---

# Pax8 + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Pax8
> API. Not affiliated with, endorsed by, or sponsored by Pax8, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

MSPs resell Microsoft, security, and backup through Pax8, then lose days each month reconciling its invoices against subscriptions and their PSA. Ask your AI "where is billing leaking this month," "what's my MRR and margin," or "which usage is about to overage," and get answers the Pax8 portal can't compose: invoices joined to subscriptions joined to usage, computed offline from a local mirror in one query instead of a CSV export and a spreadsheet.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/pax8){: .btn}

## Instead of clicking through Pax8, just ask

**Instead of** Exporting the monthly Pax8 invoice to a spreadsheet and matching each line against your active subscriptions by hand to find what is billing for a cancelled product or was never invoiced at all
**just ask:** *"Where is my Pax8 billing leaking this month?"*
<sub>Your agent runs: <code>pax8-cli reconcile</code></sub>

**Instead of** Building a spreadsheet of subscription price times quantity across every customer, subtracting partner cost, to get a revenue and margin number the portal will not total for you
**just ask:** *"What is my recurring revenue and margin right now, by product?"*
<sub>Your agent runs: <code>pax8-cli mrr</code></sub>

**Instead of** Waiting for the metered-usage line to land on a customer invoice before you discover that Azure or backup consumption spiked, then eating the margin hit
**just ask:** *"Which customers are about to blow past their usual usage?"*
<sub>Your agent runs: <code>pax8-cli overage</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/pax8/wide-1200x630.png" src="/assets/video/pax8/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/pax8/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Where is my billing leaking - invoiced for a cancelled product, or active but never billed? | `pax8-cli reconcile` |
| Can I catch that leakage before the next invoice finalizes? | `pax8-cli reconcile --draft` |
| What is my MRR and margin right now, broken down by product? | `pax8-cli mrr` |
| Which usage summaries are running hot before they hit the invoice? | `pax8-cli overage` |
| What changed in my book of business this week - new, cancelled, resized subscriptions? | `pax8-cli since 7d` |
| Which customers cost the most across every invoice? | `pax8-cli spend` |
| Everything about one customer - subscriptions, contacts, invoices, usage - in one view? | `pax8-cli company show <companyId>` |
| Which products can I resell that match a vendor or keyword? | `pax8-cli search "microsoft 365"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/pax8/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/pax8/guide.md).

## What makes this one different

Most Pax8 integrations and MCP servers proxy each question into a live Partner API call - fine for fetching one subscription, but a reconciliation or MRR question becomes a multi-call dance across invoices, subscriptions, products, and usage that the AI burns context stitching together. This skill syncs Pax8 into a local SQLite mirror with full-text search, so the cross-entity questions - billing leakage, MRR and margin, usage overages, customer-360 - become one local join: instant, offline, and the AI sees the answer, not pages of raw invoice JSON.

It complements the Pax8 marketplace rather than replacing it: the portal stays the place to provision and order, while this skill brings billing reconciliation, MRR, margin, and usage-overage detection - the cross-entity math no single portal screen composes - to whichever AI agent you already use, computed offline from your own synced mirror.

## The pain this closes

- Reconciling Pax8 invoices is a multi-day monthly slog. One MSP profiled by automation vendor Bumblebee spent five days every month matching Pax8 invoices against their PSA before automating it, because the marketplace has no single view that joins invoice lines back to active subscriptions.
- A whole market of paid add-ons exists just to reconcile Pax8 billing, which is the clearest proof the portal does not surface billing leakage, MRR, or margin in one place. Pax8's own developer documentation warns that mapping invoice line items back to subscriptions, and handling credited invoices, is non-trivial integration work.
- Metered-usage overages (Azure, backup, per-GB security) only become visible after they post to the customer invoice, so MSPs absorb the surprise instead of catching the spike while it is still happening.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pax8/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pax8/install.ps1 | iex
```

After install, authenticate once with your Pax8 credentials, then verify with `pax8-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | pax8-cli reconcile; pax8-cli mrr; pax8-cli overage; pax8-cli spend; pax8-cli since 7d; pax8-cli company show <companyId>; pax8-cli search "microsoft 365"; pax8-cli companies find; pax8-cli subscriptions find; pax8-cli invoices find-partner | Allow |
| Write (routine) | pax8-cli companies create-company; pax8-cli companies update-company; pax8-cli companies contacts create; pax8-cli companies contacts update; pax8-cli subscriptions update; pax8-cli orders create - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | pax8-cli subscriptions delete (cancels a subscription); pax8-cli companies contacts delete | Human-in-the-loop only |

The skill drives the pax8-cli and pax8-mcp binaries, authenticating with a Pax8 Partner API OAuth2 client ID and secret read from the environment (PAX8_CLIENT_ID, PAX8_CLIENT_SECRET) - never logged and never sent anywhere except Pax8's own API. Read commands (reconcile, mrr, overage, spend, since, company show, search, and the find/get queries) change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep subscription cancellation, contact deletion, and order creation human-only. The strongest control is the permission scope on the API credential you create. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/pax8/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Pax8 MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Pax8 data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Pax8 partner, and what credentials does it use?

Yes - it talks to the Pax8 Partner API with your own partner credentials. Create an OAuth2 Client ID and Client Secret in the Pax8 portal under Integrations, then set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET. The CLI exchanges them for a bearer token at Pax8's token endpoint and caches it. PAX8_AUDIENCE (default api://p8p.client) and PAX8_OAUTH_SCOPE are optional overrides. The credential's own permissions are the real boundary - scope it to what you want the AI to reach.

### Will this hit my Pax8 API rate limits?

After the first sync, the analytics commands (reconcile, mrr, overage, spend, since, company show, search) run against your local SQLite mirror with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Does this replace the Pax8 portal?

No. Provisioning, ordering, and support workflows stay in the portal. This skill answers the cross-entity billing and revenue questions the portal cannot compose in one place, from your terminal or agent.


## Status

Beta. Validated against the Pax8 API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
