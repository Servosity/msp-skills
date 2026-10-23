---
layout: default
title: "TD SYNNEX StreamOne ION MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every StreamOne ION reseller API, plus a local licence book for renewals, margin and cross-distributor true-up."
permalink: /skills/td-synnex/
skill_name: "TD SYNNEX StreamOne ION MCP"
image: /assets/social/td-synnex/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for TD SYNNEX StreamOne ION?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for TD SYNNEX StreamOne ION, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the TD SYNNEX StreamOne ION MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `TD_SYNNEX_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `TD_SYNNEX_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running td-synnex-mcp in HTTP mode, and in this build HTTP mode checks a token but has no request size or time limits yet. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You paste your ION refresh token once."
  - q: "Is my TD SYNNEX data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "How does it sign in to StreamOne ION?"
    a: "With an ION OAuth refresh token. In the ION portal open Settings > Users > Edit > OAuth credentials > Request New Credentials, then pipe the refresh token to td-synnex-cli auth mint --refresh-token-stdin and set TD_SYNNEX_ACCOUNT_ID to your ION account ID. ION refresh tokens are single-use and last 32 days, and access tokens last 2 hours. auth mint saves the new pair to a private credentials file before it prints anything, and running td-synnex-cli auth mint --if-expiring 15m on a schedule keeps it fresh. td-synnex-cli auth check shows how long both tokens stay valid."
  - q: "Will this hit my ION API limits?"
    a: "subscriptions renewals, subscriptions changes, margin, position export, position trueup, orders stuck and customers show answer from the local store. When the store is stale they first refresh the first page (about 100 rows) of each resource, so with a larger book run td-synnex-cli sync before whole-book answers; add --data-source local (or set TD_SYNNEX_NO_AUTO_REFRESH=1) to never call the API from them. Live calls respect the --rate-limit throttle."
  - q: "Can it change subscriptions or place orders?"
    a: "Yes, and that is why the safety model matters. subscriptions set previews the UPDATE order and places it only with --confirm. Other writes (carts checkout, customer-orders create-order and cancel-order, customers create and update) send as soon as they run; preview them with --dry-run first. Keep orders, cancellations and suspensions human-approved."
  - q: "Are the margin numbers exact?"
    a: "Not yet proven. margin, subscriptions renewals, position export and customers show use the price and cost ION reports for each subscription and do not multiply them by seats (an amount ION does not send is reported as unknown, never as 0), and no MSP has yet confirmed the connector against a live ION tenant. Check one multi-seat subscription against the ION portal before you rely on the totals."
  - q: "Can I run the Pax8 or PSA true-up from Claude Desktop or ChatGPT?"
    a: "Not the file comparison. position trueup --with and --billed read export files from your disk, so they run where your agent has a shell (Claude Code, Codex, Cowork) or in your terminal. Over MCP the tool refuses file paths, because a tool that reads any path an agent names could read your credentials file, and it only lists SKUs a customer holds in more than one TD SYNNEX subscription."
  - q: "Does this replace the ION portal?"
    a: "No. Ordering, provisioning and invoicing stay in StreamOne ION. This skill answers the questions that span every customer, from your terminal or agent."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your TD SYNNEX StreamOne ION credentials once, then run td-synnex-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a TD SYNNEX StreamOne ION question in plain language; it runs td-synnex-cli for you."
---

# The TD SYNNEX StreamOne ION MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by TD SYNNEX Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for TD SYNNEX StreamOne ION. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects TD SYNNEX StreamOne ION to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs buying Microsoft 365, Azure and security through TD SYNNEX StreamOne ION check renewals, margin and seat counts one customer at a time. Ask your AI "what renews in the next 30 days with auto-renew off," "which customers am I selling at a loss," or "do my TD SYNNEX and Pax8 seats match what I bill," and get the answer from a local copy of your ION book in one query.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/td-synnex){: .btn}

## Instead of clicking through TD SYNNEX StreamOne ION, just ask

**Instead of** Scrolling the ION subscriptions list and checking each NCE term's end date and auto-renew setting by hand, before Microsoft's seven-day window to reduce seats closes
**just ask:** *"What renews in the next 30 days, and which of those have auto-renew off?"*
<sub>Your agent runs: <code>td-synnex-cli subscriptions renewals --within 30d</code></sub>

**Instead of** Exporting ION subscriptions to a spreadsheet and subtracting cost from price per customer to find the accounts you sell below cost
**just ask:** *"Which TD SYNNEX customers am I losing money on?"*
<sub>Your agent runs: <code>td-synnex-cli margin --by customer</code></sub>

**Instead of** Lining up TD SYNNEX, Pax8 and PSA exports by hand to find seats you pay a distributor for but never bill the client
**just ask:** *"Do my TD SYNNEX and Pax8 seats match what the PSA bills?"*
<sub>Your agent runs: <code>td-synnex-cli position trueup --with pax8-positions.csv --billed psa-agreements.csv --map quantity=seats --map company=customer_name</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/td-synnex/wide-1200x630.png" src="/assets/video/td-synnex/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/td-synnex/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which subscriptions renew in the next 30 days with auto-renew switched off? | `td-synnex-cli subscriptions renewals --within 30d --auto-renew off` |
| Which customers am I selling at a loss or below list price? | `td-synnex-cli margin --by customer` |
| Which SKUs carry the thinnest margin across every customer? | `td-synnex-cli margin --by sku` |
| Do my TD SYNNEX and Pax8 seats agree for each customer and SKU? | `td-synnex-cli position trueup --with pax8-positions.csv --map company=customer_name --map sku=sku_key --map quantity=seats` |
| Am I billing every seat I pay TD SYNNEX for? | `td-synnex-cli position trueup --billed psa-agreements.csv --map billed_qty=seats` |
| Which orders are stuck on hold or failed this week? | `td-synnex-cli orders stuck --older-than 24h --since 7d` |
| What seat, renewal and status changes went through ION in the last week? | `td-synnex-cli subscriptions changes --since 7d` |
| What does one customer hold, what does it cost, and what is still open? | `td-synnex-cli customers show "Contoso"` |
| What would changing this subscription to 12 seats send to ION? | `td-synnex-cli subscriptions set "10505" "108536" "1608355" --seats 12` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/td-synnex/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/td-synnex/guide.md).

## What makes this one different

Most StreamOne ION integrations are PSA billing connectors or API wrappers that make one live call per question. This skill syncs your ION customers, subscriptions and orders into a local SQLite store once, so renewals across the whole book, margin by customer or SKU, stuck orders, and a seat true-up against Pax8, Sherweb or PSA exports each become one local query: offline, and the AI sees the answer instead of pages of order JSON.

It complements the StreamOne ION portal and TD SYNNEX's PSA connectors rather than replacing them: ordering, provisioning and billing sync stay there, and this skill brings the cross-customer questions (renewal windows, margin ranking, multi-distributor seat true-up) to the AI agent you already use.

## The pain this closes

- Microsoft's New Commerce terms only let you cut seats or cancel in the first seven days of a term or a renewal. Miss the renewal and you carry another year of seats the client may no longer pay for. The tminus365 blog for MSPs ("How I automated subscription renewal notices for Microsoft NCE", 2022) describes building a Power Automate flow on a distributor API just to open a ticket before each renewal, because the portal does not put the whole book's renewals in front of you.
- Price and cost live on each subscription, so finding the customers and SKUs sold at a loss, or below list price, means exporting the book and rebuilding the math in a spreadsheet every month.
- Many MSPs buy from more than one distributor. Seat counts then sit in TD SYNNEX, in Pax8 or Sherweb, and in the PSA agreement, and lining them up per customer and SKU to catch unbilled seats is manual work unless you pay for a separate billing-reconciliation platform.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/td-synnex/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/td-synnex/install.ps1 | iex
```

After install, authenticate once with your TD SYNNEX StreamOne ION credentials, then verify with `td-synnex-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | td-synnex-cli subscriptions renewals; td-synnex-cli subscriptions changes; td-synnex-cli margin; td-synnex-cli position export; td-synnex-cli position trueup; td-synnex-cli orders stuck; td-synnex-cli customers show "Contoso"; td-synnex-cli subscriptions list; td-synnex-cli customers list; td-synnex-cli products list; td-synnex-cli pricing; td-synnex-cli cloud-profiles; td-synnex-cli reports get-data; td-synnex-cli sync; td-synnex-cli auth check | Allow |
| Write (routine) | td-synnex-cli customers create; td-synnex-cli customers update; td-synnex-cli carts create; td-synnex-cli carts update; td-synnex-cli cart-items create; td-synnex-cli cart-items update; td-synnex-cli reports save. These send immediately; --dry-run is an opt-in preview | Preview with --dry-run, then a reviewed write |
| Orders and spend | td-synnex-cli subscriptions set (seats, renewal setting, friendly name; places an order only with --confirm); td-synnex-cli carts checkout; td-synnex-cli customer-orders create-order; td-synnex-cli cloud-providers provision-customer; td-synnex-cli cloud-providers link-customer; td-synnex-cli products enable; td-synnex-cli products disable; td-synnex-cli programs batch-update-product | Human-in-the-loop only |
| Destructive / credential | td-synnex-cli subscriptions set --status suspended or deleted; td-synnex-cli subscriptions set --renewal cancel_at_renewal (or its alias auto-off); td-synnex-cli customer-orders cancel-order; td-synnex-cli carts delete; td-synnex-cli cart-items delete; td-synnex-cli reports delete; td-synnex-cli auth mint, auth set-token and auth logout (credential changes) | Human-in-the-loop only, explicit confirmation |

The skill drives td-synnex-cli and td-synnex-mcp with an ION access token minted from your refresh token and kept in a private credentials file, or read from TD_SYNNEX_ACCESS_TOKEN; tokens are never printed unless you ask and are sent only to the ION API (ion.tdsynnex.com unless you override TD_SYNNEX_BASE_URL). The licence-book commands and every list and get read change nothing. Writes are not gated by default except subscriptions set, which needs --confirm: treat ordering, checkout, cancellation, suspension and deletion as human-only, and set TD_SYNNEX_MCP_READ_ONLY=1 to keep the MCP server to reads. The strongest control is the ION user whose OAuth credentials you issue. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/td-synnex/governance.md).

## Frequently asked questions

### Is there an MCP server for TD SYNNEX StreamOne ION?

Yes - this one. A free, open source MCP server and Claude Code Skill for TD SYNNEX StreamOne ION, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the TD SYNNEX StreamOne ION MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `TD_SYNNEX_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `TD_SYNNEX_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running td-synnex-mcp in HTTP mode, and in this build HTTP mode checks a token but has no request size or time limits yet. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You paste your ION refresh token once.

### Is my TD SYNNEX data safe?

Your data stays on your machine. The CLI, MCP server, and the local store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### How does it sign in to StreamOne ION?

With an ION OAuth refresh token. In the ION portal open Settings > Users > Edit > OAuth credentials > Request New Credentials, then pipe the refresh token to td-synnex-cli auth mint --refresh-token-stdin and set TD_SYNNEX_ACCOUNT_ID to your ION account ID. ION refresh tokens are single-use and last 32 days, and access tokens last 2 hours. auth mint saves the new pair to a private credentials file before it prints anything, and running td-synnex-cli auth mint --if-expiring 15m on a schedule keeps it fresh. td-synnex-cli auth check shows how long both tokens stay valid.

### Will this hit my ION API limits?

subscriptions renewals, subscriptions changes, margin, position export, position trueup, orders stuck and customers show answer from the local store. When the store is stale they first refresh the first page (about 100 rows) of each resource, so with a larger book run td-synnex-cli sync before whole-book answers; add --data-source local (or set TD_SYNNEX_NO_AUTO_REFRESH=1) to never call the API from them. Live calls respect the --rate-limit throttle.

### Can it change subscriptions or place orders?

Yes, and that is why the safety model matters. subscriptions set previews the UPDATE order and places it only with --confirm. Other writes (carts checkout, customer-orders create-order and cancel-order, customers create and update) send as soon as they run; preview them with --dry-run first. Keep orders, cancellations and suspensions human-approved.

### Are the margin numbers exact?

Not yet proven. margin, subscriptions renewals, position export and customers show use the price and cost ION reports for each subscription and do not multiply them by seats (an amount ION does not send is reported as unknown, never as 0), and no MSP has yet confirmed the connector against a live ION tenant. Check one multi-seat subscription against the ION portal before you rely on the totals.

### Can I run the Pax8 or PSA true-up from Claude Desktop or ChatGPT?

Not the file comparison. position trueup --with and --billed read export files from your disk, so they run where your agent has a shell (Claude Code, Codex, Cowork) or in your terminal. Over MCP the tool refuses file paths, because a tool that reads any path an agent names could read your credentials file, and it only lists SKUs a customer holds in more than one TD SYNNEX subscription.

### Does this replace the ION portal?

No. Ordering, provisioning and invoicing stay in StreamOne ION. This skill answers the questions that span every customer, from your terminal or agent.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [AWS](/skills/aws-billing/) · [DataGate](/skills/datagate/) · [Gradient MSP](/skills/gradient/) · [Maxio](/skills/maxio/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the TD SYNNEX StreamOne ION API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
