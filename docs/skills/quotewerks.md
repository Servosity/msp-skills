---
layout: default
title: "QuoteWerks MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Typed quote tools for QuoteWerks Web that use the real column names, preview typed writes before sending them, and add margin, win-rate and pipeline answers from a local mirror."
permalink: /skills/quotewerks/
skill_name: "QuoteWerks MCP"
image: /assets/social/quotewerks/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for QuoteWerks?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for QuoteWerks, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the QuoteWerks MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `QUOTEWERKS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `QUOTEWERKS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Your API key stays in your environment unless you save it with `quotewerks-cli auth set-token`, which writes it to the CLI's credentials file (mode 0600) until you run `quotewerks-cli auth logout`, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but quotewerks-mcp speaks HTTP natively: set `QUOTEWERKS_MCP_HTTP_TOKEN` to a bearer token, run `quotewerks-mcp --transport http`, and put its /mcp endpoint (http://127.0.0.1:7777/mcp by default) behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your API key once."
  - q: "Is my QuoteWerks data safe?"
    a: "Your data stays on your machine. The CLI, the MCP server and the local mirror all run locally, and the AI sees query results, not a bulk dump of your quotes. MSP Skills never bundles or transmits your API key."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does it work with QuoteWerks Desktop?"
    a: "It works with QuoteWerks Web through the QuoteWerks Web REST API at qwwapi.quotewerks.com, which QuoteWerks labels beta. It does not open a QuoteWerks Desktop database or use the desktop COM API."
  - q: "What API key do I need, and can the AI change my quotes?"
    a: "Create a key in QuoteWerks Web under Setup > Options > Zapier and API; the API requires a user with MasterRights. A key without Allow write access can read but cannot change anything (QuoteWerks answers writes with 403). Even with a write-enabled key, `docs create`, `items add`, `items reprice` and `requote` only preview until you add `--confirm`. The raw `tables insert`, `tables update` and `tables delete` commands send immediately, so keep the key read-only unless you need writes."
  - q: "Will this hammer the QuoteWerks API?"
    a: "No. margin, winrate, pipeline, cost-drift and changes read the local copy that `quotewerks-cli mirror` makes, so you can ask as many questions as you like without another API call. mirror reads 1,000 rows per request, and requests are paced to the server's rate-limit headers by default."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your QuoteWerks credentials once, then run quotewerks-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a QuoteWerks question in plain language; it runs quotewerks-cli for you."
---

# The QuoteWerks MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Aspire Technologies, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for QuoteWerks. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects QuoteWerks to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI where a quote stands, what is on it, and whether any line sells below cost, then have it build or reprice the quote for you to review before anything is written. quotewerks-cli works against QuoteWerks Web using the real column names, previews every typed write, and keeps a local copy of your documents so margin, win-rate, pipeline and cost-drift questions are one command each.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/quotewerks){: .btn}

## Instead of clicking through QuoteWerks, just ask

**Instead of** Open QuoteWerks, run a Document Lookup to find what the client bought in March, then hand-key every line into a new quote.
**just ask:** *"Copy quote 1042 into a new quote with 12 of the laptops at today's prices, and show me before you create it."*
<sub>Your agent runs: <code>quotewerks-cli requote "1042" --qty 20UES0X100=12</code></sub>

**Instead of** When a distributor changes cost, open every open quote and check each line on the Document Items tab.
**just ask:** *"Which open quotes have lines whose cost moved at least 2% since we quoted them, and what does it do to margin?"*
<sub>Your agent runs: <code>quotewerks-cli cost-drift --threshold 2</code></sub>

**Instead of** Write a one-off script or build a report every time someone asks for margin by rep before a QBR.
**just ask:** *"What was our margin by sales rep on orders over the last 90 days?"*
<sub>Your agent runs: <code>quotewerks-cli margin --by rep --since 90d</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/quotewerks/wide-1200x630.png" src="/assets/video/quotewerks/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/quotewerks/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Where does the Acme quote stand? | `quotewerks-cli docs search --customer "Acme" --type QUOTE` |
| What is on quote 1042, line by line, with margin per line? | `quotewerks-cli docs get "1042"` |
| Is anything on this quote priced below cost or under 10% margin before it goes out? | `quotewerks-cli docs lint "1042" --min-margin 10` |
| What does an 18% margin on these laptops do to the quote? Show me before you change it. | `quotewerks-cli items reprice "1042" --part 20UES0X100 --margin 18` |
| Which open quotes are 30 days old or more, and who do I chase? | `quotewerks-cli pipeline --older-than 30d` |
| What is our quote win rate by rep over the last 90 days? | `quotewerks-cli winrate --by rep --since 90d` |
| Which open quotes carry a cost that no longer matches our product database? | `quotewerks-cli cost-drift --threshold 2` |
| What moved in the quote pipeline this week? | `quotewerks-cli changes --since 7d` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/quotewerks/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/quotewerks/guide.md).

## What makes this one different

The QuoteWerks Web REST API is generic table access, and the [Zapier QuoteWerks app](https://zapier.com/apps/quotewerks/integrations) works one record at a time (create a quote, create a line item, find a quote, react to a new document). This skill adds typed quote commands that use the real QuoteWerks column names, fills new lines from your product databases, previews every typed write before it is sent, and keeps a local copy of your documents so cross-document margin, win-rate, pipeline and cost-drift questions are one local query.

QuoteWerks Web has its own AI agent, and this skill can ask it questions too (`agent ask`). What the skill adds is reach from the AI you already work in: previewed quote builds and repricing, a per-line audit before a quote goes out, and analysis across every mirrored document. It complements QuoteWerks Web; it does not replace it.

## The pain this closes

- Quoting from an inbound email ("12 more laptops like the ones in March, plus docks") means finding the old document and re-keying every line into a new one. An MSP account manager described exactly this in a request on the QuoteWerks support forum.
- Answering "where does the Acme quote stand?" means leaving what you are doing to run a Document Lookup in QuoteWerks.
- When distributor cost moves, every affected quote has to be opened and each line touched by hand, and the quotes nobody touched go out at a thinner margin than anyone intended.
- Margin and win-rate questions for a QBR mean writing a script or building a report for each question.
- The QuoteWerks Web REST API (beta) is generic table access. An AI handed a raw table endpoint has to guess the column names, and a wrong guess quietly produces a bad quote.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quotewerks/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex
```

After install, authenticate once with your QuoteWerks credentials, then verify with `quotewerks-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `docs search`, `docs get`, `docs lint`, `items list`, `products search`, the `tables` reads (`list`, `rows`, `row`, `query`, `schema`, `values`), the `cloud-settings` reads, `reports run`, `identity userinfo`, `agent ask` (and `import agent`, CLI only, which sends a JSONL file of questions to the same AI agent), and the mirror reports (`margin`, `pipeline`, `cost-drift`, `winrate`, `changes`); `mirror` and `sync` write only the local copy | Allow |
| Previewed writes | `docs create`, `items add`, `items reprice`, `requote`: each previews by default and writes only with `--confirm` and a write-enabled key | Let the agent preview; a human approves before it re-runs with --confirm |
| Raw writes | `tables insert` and `tables update` (CLI only, never an MCP tool), which send immediately to any table your key can write | Preview with --dry-run, then a reviewed write |
| Destructive / credential | `tables delete` (CLI only; deletes a row in any table, documents included), `auth set-token`, `auth logout` | Human-in-the-loop only |

The skill reads your QuoteWerks Web quotes, orders, invoices, line items, product databases and settings, and it can create and change documents. The typed writes (`docs create`, `items add`, `items reprice`, `requote`) only preview until you add `--confirm`. The raw table writes (`tables insert`, `tables update`) send immediately, so preview them with `--dry-run` first, and keep `tables delete` and the credential commands for a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/quotewerks/governance.md).

## Frequently asked questions

### Is there an MCP server for QuoteWerks?

Yes - this one. A free, open source MCP server and Claude Code Skill for QuoteWerks, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the QuoteWerks MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `QUOTEWERKS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `QUOTEWERKS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Your API key stays in your environment unless you save it with `quotewerks-cli auth set-token`, which writes it to the CLI's credentials file (mode 0600) until you run `quotewerks-cli auth logout`, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but quotewerks-mcp speaks HTTP natively: set `QUOTEWERKS_MCP_HTTP_TOKEN` to a bearer token, run `quotewerks-mcp --transport http`, and put its /mcp endpoint (http://127.0.0.1:7777/mcp by default) behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your API key once.

### Is my QuoteWerks data safe?

Your data stays on your machine. The CLI, the MCP server and the local mirror all run locally, and the AI sees query results, not a bulk dump of your quotes. MSP Skills never bundles or transmits your API key.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does it work with QuoteWerks Desktop?

It works with QuoteWerks Web through the QuoteWerks Web REST API at qwwapi.quotewerks.com, which QuoteWerks labels beta. It does not open a QuoteWerks Desktop database or use the desktop COM API.

### What API key do I need, and can the AI change my quotes?

Create a key in QuoteWerks Web under Setup > Options > Zapier and API; the API requires a user with MasterRights. A key without Allow write access can read but cannot change anything (QuoteWerks answers writes with 403). Even with a write-enabled key, `docs create`, `items add`, `items reprice` and `requote` only preview until you add `--confirm`. The raw `tables insert`, `tables update` and `tables delete` commands send immediately, so keep the key read-only unless you need writes.

### Will this hammer the QuoteWerks API?

No. margin, winrate, pipeline, cost-drift and changes read the local copy that `quotewerks-cli mirror` makes, so you can ask as many questions as you like without another API call. mirror reads 1,000 rows per request, and requests are paced to the server's rate-limit headers by default.


## More CRM connectors

Run more than one CRM tool, or comparing options? These connectors work the same way: [HubSpot](/skills/hubspot/) · [Pipedrive](/skills/pipedrive/) · [Salesbuildr](/skills/salesbuildr/)

## Status

Beta. Validated against the QuoteWerks API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
