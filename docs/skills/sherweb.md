---
layout: default
title: "Sherweb MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Sherweb Partner API capability, plus a local SQLite store, offline analytics, and margin/drift/orphan joins no other Sherweb tool has."
permalink: /skills/sherweb/
skill_name: "Sherweb MCP"
image: /assets/social/sherweb/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Sherweb?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Sherweb, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Sherweb MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SHERWEB_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but sherweb-mcp speaks HTTP natively: run `sherweb-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Sherweb data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Sherweb partner, and what credentials does it use?"
    a: "Yes - it talks to the Sherweb Partner API with your own partner credentials, using composed authentication. You need an OAuth2 client-credentials Client ID and Secret (with a scope) for the bearer token, plus an APIM gateway subscription key that rides on every call. Create the OAuth2 application and copy the subscription key from cumulus.sherweb.com under Security > APIs, then set SHERWEB_CLIENT_ID, SHERWEB_CLIENT_SECRET, SHERWEB_OAUTH_SCOPE, and SHERWEB_SUBSCRIPTION_KEY. The credential's own permissions are the real boundary - scope it to what you want the AI to reach. Run sherweb-cli doctor to confirm auth and connectivity."
  - q: "Will this hit my Sherweb API rate limits?"
    a: "After deep-sync, the analytics commands (margin, margin-trend, orphans, usage-leak, right-size, drift, sub-changes, fleet-subs, amend-preview) run against your local SQLite mirror with zero API calls. Live calls respect a --rate-limit throttle, and sync is resumable and incremental - it only fetches what changed since the last checkpoint."
  - q: "Does this replace the Sherweb portal?"
    a: "No. Provisioning, ordering, and subscription management stay in the portal. This skill answers the cross-entity margin and billing questions the portal cannot compose in one place, from your terminal or agent."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Sherweb credentials once, then run sherweb-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Sherweb question in plain language; it runs sherweb-cli for you."
---

# The Sherweb MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Sherweb Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Sherweb. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Sherweb to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs resell Microsoft 365, Azure, and security through Sherweb, then spend the monthly close reconciling what they owe Sherweb against what they bill customers. Ask your AI "what's my net margin per customer," "which subscriptions am I paying for but not billing," or "what will this seat change cost," and get answers the Sherweb portal cannot compose: payable charges joined to receivable charges and subscriptions, computed offline from a local mirror in one query instead of a CSV export and a spreadsheet.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/sherweb){: .btn}

## Instead of clicking through Sherweb, just ask

**Instead of** Exporting Sherweb's payable charges and each customer's receivable charges into a spreadsheet and matching them line by line to find out who is actually making you money this month
**just ask:** *"What is my net margin per customer this month?"*
<sub>Your agent runs: <code>sherweb-cli margin</code></sub>

**Instead of** Scrolling every customer's subscription list to find active seats you still pay Sherweb for but stopped billing the client months ago
**just ask:** *"Which subscriptions am I paying for but not billing back?"*
<sub>Your agent runs: <code>sherweb-cli orphans</code></sub>

**Instead of** Guessing what a mid-term seat increase will add to next month's invoice, then finding out the hard way when the bill lands
**just ask:** *"What will bumping this customer to 25 seats cost before I commit it?"*
<sub>Your agent runs: <code>sherweb-cli amend-preview --sub "SUB123" --qty 25</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/sherweb/wide-1200x630.png" src="/assets/video/sherweb/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/sherweb/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What is my net margin per customer this month - receivable minus payable? | `sherweb-cli margin --month 2026-04` |
| Whose margin is sliding month over month before an account goes negative? | `sherweb-cli margin-trend --last 6` |
| Which active subscriptions am I paying Sherweb for but not billing the customer? | `sherweb-cli orphans` |
| Where am I absorbing metered usage I never billed back? | `sherweb-cli usage-leak` |
| Which subscriptions have more seats paid than seats actually used? | `sherweb-cli right-size` |
| What changed on my payable charges since the last sync - new, vanished, or repriced? | `sherweb-cli drift` |
| What subscriptions were added, cancelled, or resized across my whole book this month? | `sherweb-cli sub-changes --since 30d` |
| How many total seats of each product do I carry across every customer? | `sherweb-cli fleet-subs --product "Microsoft 365"` |
| What will a seat change cost before I actually submit the amendment? | `sherweb-cli amend-preview --sub "SUB123" --qty 25` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/sherweb/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/sherweb/guide.md).

## What makes this one different

Most Sherweb integrations and MCP servers proxy each question into a live Partner API call - fine for fetching one customer, but a margin or reconciliation question becomes a multi-call dance across the Distributor billing endpoints, per-customer receivable charges, subscriptions, and usage that an AI burns context stitching together one customer at a time. This skill runs deep-sync once into a local SQLite mirror, so the cross-entity questions - net margin, orphaned subscriptions, usage leakage, seat right-sizing - become a single local join: instant, offline, and the AI sees the answer, not pages of raw charge JSON.

It complements the Sherweb partner portal rather than replacing it: the portal stays where you provision, order, and manage subscriptions, while this skill brings the cross-entity math no single portal screen composes - net margin per customer, margin trend, orphaned and under-billed subscriptions, usage leakage - to whichever AI agent you already use, computed offline from your own synced mirror.

## The pain this closes

- Sherweb's Partner API splits billing in two: the Distributor API returns the payable charges you owe Sherweb, while the Service Provider API returns the receivable charges and subscriptions you bill customers. No single portal screen joins them, so the one number an MSP owner actually wants - net margin per customer - gets rebuilt by hand in a spreadsheet every month.
- Unbilled and orphaned licenses are a recurring r/msp complaint: a customer offboards or downsizes, the subscription stays active on the Sherweb side, and the MSP keeps paying for seats it never bills back - margin quietly bleeding until someone audits the whole book.
- Metered platform usage (Azure consumption, per-GB add-ons) only surfaces once it posts to a charge, so over-provisioned seats and absorbed consumption stay invisible until the close - by which point the margin hit is already taken.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sherweb/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sherweb/install.ps1 | iex
```

After install, authenticate once with your Sherweb credentials, then verify with `sherweb-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | sherweb-cli margin; sherweb-cli margin-trend --last 6; sherweb-cli orphans; sherweb-cli usage-leak; sherweb-cli right-size; sherweb-cli drift; sherweb-cli sub-changes --since 30d; sherweb-cli fleet-subs; sherweb-cli amend-preview --sub "SUB123" --qty 25; sherweb-cli distributor; sherweb-cli service-provider list-customers; sherweb-cli service-provider get-receivable-charges; sherweb-cli service-provider validate-order; sherweb-cli sync; sherweb-cli deep-sync | Allow |
| Write (routine) | sherweb-cli service-provider amend-subscriptions (submits a seat-quantity change); sherweb-cli service-provider place-order (places a marketplace order); sherweb-cli import "<resource>" - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | sherweb-cli service-provider cancel-subscriptions (cancels a customer's subscriptions); sherweb-cli auth login, sherweb-cli auth logout, and sherweb-cli auth set-token (credential changes) | Human-in-the-loop only |

The skill drives the sherweb-cli and sherweb-mcp binaries, authenticating with Sherweb Partner API credentials read from the environment (SHERWEB_CLIENT_ID, SHERWEB_CLIENT_SECRET, SHERWEB_OAUTH_SCOPE, SHERWEB_SUBSCRIPTION_KEY) - never logged and never sent anywhere except Sherweb's own API. The analytics and list/get read commands (margin, margin-trend, orphans, usage-leak, right-size, drift, sub-changes, fleet-subs, amend-preview, distributor, and the service-provider list/get queries) change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep subscription amendments, order placement, and especially subscription cancellation human-only, and treat the auth commands as credential operations. The strongest control is the permission scope on the OAuth2 application and subscription key you create. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/sherweb/governance.md).

## Frequently asked questions

### Is there an MCP server for Sherweb?

Yes - this one. A free, open source MCP server and Claude Code Skill for Sherweb, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Sherweb MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SHERWEB_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but sherweb-mcp speaks HTTP natively: run `sherweb-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Sherweb data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Sherweb partner, and what credentials does it use?

Yes - it talks to the Sherweb Partner API with your own partner credentials, using composed authentication. You need an OAuth2 client-credentials Client ID and Secret (with a scope) for the bearer token, plus an APIM gateway subscription key that rides on every call. Create the OAuth2 application and copy the subscription key from cumulus.sherweb.com under Security > APIs, then set SHERWEB_CLIENT_ID, SHERWEB_CLIENT_SECRET, SHERWEB_OAUTH_SCOPE, and SHERWEB_SUBSCRIPTION_KEY. The credential's own permissions are the real boundary - scope it to what you want the AI to reach. Run sherweb-cli doctor to confirm auth and connectivity.

### Will this hit my Sherweb API rate limits?

After deep-sync, the analytics commands (margin, margin-trend, orphans, usage-leak, right-size, drift, sub-changes, fleet-subs, amend-preview) run against your local SQLite mirror with zero API calls. Live calls respect a --rate-limit throttle, and sync is resumable and incremental - it only fetches what changed since the last checkpoint.

### Does this replace the Sherweb portal?

No. Provisioning, ordering, and subscription management stay in the portal. This skill answers the cross-entity margin and billing questions the portal cannot compose in one place, from your terminal or agent.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [AWS](/skills/aws-billing/) · [Gradient MSP](/skills/gradient/) · [Maxio](/skills/maxio/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the Sherweb API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
