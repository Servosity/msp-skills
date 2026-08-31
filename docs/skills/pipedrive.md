---
layout: default
title: "Pipedrive MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Full Pipedrive CRUD plus a local SQLite pipeline copy: stale deals, forecasts, aging, dupes, rep leaderboards."
permalink: /skills/pipedrive/
skill_name: "Pipedrive MCP"
image: /assets/social/pipedrive/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Pipedrive?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Pipedrive, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Pipedrive MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PIPEDRIVE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PIPEDRIVE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but pipedrive-mcp speaks HTTP natively: run `pipedrive-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Pipedrive data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this need a higher Pipedrive plan or the paid Insights tier?"
    a: "No. Any Pipedrive plan that issues an API token works - find it under Settings > Personal preferences > API. The skill talks to the standard Pipedrive API, and the cross-entity analytics (stale, forecast, aging, leaderboard) run locally on your synced data, so they are not gated behind a reporting add-on."
  - q: "Will this burn through my Pipedrive API rate limits?"
    a: "Day-to-day questions answer from the local mirror after a `sync`, so they make zero API calls. `sync` itself paginates politely - tune `--rate-limit` and `--concurrency`, and use `--since 24h` to refresh only what changed - and live calls only happen when local data is missing or stale."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pipedrive/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pipedrive/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Pipedrive credentials once, then run pipedrive-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Pipedrive question in plain language; it runs pipedrive-cli for you."
---

# The Pipedrive MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Pipedrive OÜ.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Pipedrive. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Pipedrive to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which deals are dying and who do I call today," and get a ranked answer in seconds. The Pipedrive skill keeps a local copy of your pipeline so it can join deals, people, activities, and notes the portal shows on separate screens - surfacing stale deals by dollar at risk, a weighted forecast, stage bottlenecks, and rep leaderboards without exporting a single CSV.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/pipedrive){: .btn}

## Instead of clicking through Pipedrive, just ask

**Instead of** Export the deals report to a spreadsheet every Monday and eyeball which ones have gone quiet.
**just ask:** *"Which open deals has nobody touched in two weeks, worst dollar value first?"*
<sub>Your agent runs: <code>pipedrive-cli stale --quiet-days 14 --agent</code></sub>

**Instead of** Rebuild the weighted-pipeline math (deal value times stage probability) in a spreadsheet before every pipeline review.
**just ask:** *"What's my weighted forecast for this quarter by pipeline?"*
<sub>Your agent runs: <code>pipedrive-cli forecast --period this-quarter --agent</code></sub>

**Instead of** Click through five screens - person, org, deals, activities, notes - to prep for one call.
**just ask:** *"Give me the full picture on Jane Smith before my call."*
<sub>Your agent runs: <code>pipedrive-cli who "Jane Smith" --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/pipedrive/wide-1200x630.png" src="/assets/video/pipedrive/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/pipedrive/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which open deals has nobody touched in two weeks, worst dollar value first? | `pipedrive-cli stale --quiet-days 14 --agent` |
| What's my weighted forecast for this quarter, and what's expected to close? | `pipedrive-cli forecast --period this-quarter --agent` |
| Which deals are stuck in a stage longer than that stage usually takes? | `pipedrive-cli aging --agent` |
| Which open deals have no next activity scheduled? | `pipedrive-cli next-activity --missing --agent` |
| Rank my reps by won value over the last 90 days. | `pipedrive-cli leaderboard --by won-value --window 90d --agent` |
| What changed since yesterday, and who do I need to call today? | `pipedrive-cli digest --for-me --agent` |
| Which deals did we lose in the last six months, with reasons, for a re-engagement push? | `pipedrive-cli lost --since 180d --agent` |
| Find likely-duplicate organizations so I can clean up the CRM. | `pipedrive-cli dupes --entity organizations --agent` |
| Search every synced deal, person, and organization for a name. | `pipedrive-cli search "Acme Corp" --agent` |
| Pull my whole pipeline into a local copy for offline, zero-API-call analysis. | `pipedrive-cli sync --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/pipedrive/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/pipedrive/guide.md).

## What makes this one different

Most Pipedrive integrations proxy each question straight into a live API call - one deal, one page, one entity at a time, rate-limited and shaped for single lookups. This skill keeps a local SQLite mirror of your pipeline (run `sync` once), so the high-value questions - stale deals ranked by dollar at risk, a weighted forecast, stage aging, a rep leaderboard, duplicate detection - are computed by joining deals, people, activities, and stages locally. That is analysis a thin wrapper cannot do without paging the entire API on every ask.

Pipedrive's Insights dashboards live in the portal and answer the questions Pipedrive pre-built. This skill answers the ones it did not - value at risk, next-activity gaps, lost-deal re-enrollment lists, a one-card contact view - in your terminal or AI agent, joined across entities, complementing the portal rather than replacing it.

## The pain this closes

- Reporting is Pipedrive's single most-cited complaint across G2, Capterra, and Trustpilot. Insights covers the basics, but anything cross-entity - value at risk by organization, a weighted forecast by pipeline, rep contribution over a window - means exporting to a spreadsheet and rebuilding the math by hand.
- Deals die quietly. CRM contact data decays roughly 30% a year, and a deal nobody has logged an activity against just sits in the pipeline. By the time it surfaces in a review it is already cold.
- "Great for managing deals, not for managing customers." Prepping for one call means opening the person, their organization, their open deals, the last and next activity, and recent notes on separate screens and stitching them together yourself.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pipedrive/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pipedrive/install.ps1 | iex
```

After install, authenticate once with your Pipedrive credentials, then verify with `pipedrive-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | every `get`/`list` command, the local-join analytics (`stale`, `forecast`, `aging`, `digest`, `leaderboard`, `next-activity`, `lost`, `who`, `changes`, `dupes`), `search`, `export`, and `sync` (which writes only the local SQLite mirror, never Pipedrive) | Allow |
| Write (routine) | `deals add`, `deals update`, `persons add`, `organizations update`, `notes add`, `activities add`, `leads add`, and the bulk `import` - 73 routine-write commands in all | Preview with --dry-run, then a reviewed write |
| Destructive / config | `deals delete`, `persons delete`, `organizations delete` and the other 40 delete commands, plus the credential commands (`auth set-token`, `oauth get-tokens`, `oauth refresh-tokens`) | Human-in-the-loop only |

The skill can read your whole pipeline and can create, update, and delete CRM records, so treat it like any account with write access. Reads - the reports, the analytics rollups, and search - are always safe. Routine writes such as `deals add`, `deals update`, and the bulk `import` should be previewed with `--dry-run` and approved before they run. Deletes and token commands are human-in-the-loop only. The recommended agent policy is read plus previewed writes, with a human approving anything that mutates or removes data. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/pipedrive/governance.md).

## Frequently asked questions

### Is there an MCP server for Pipedrive?

Yes - this one. A free, open source MCP server and Claude Code Skill for Pipedrive, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Pipedrive MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PIPEDRIVE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PIPEDRIVE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but pipedrive-mcp speaks HTTP natively: run `pipedrive-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Pipedrive data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this need a higher Pipedrive plan or the paid Insights tier?

No. Any Pipedrive plan that issues an API token works - find it under Settings > Personal preferences > API. The skill talks to the standard Pipedrive API, and the cross-entity analytics (stale, forecast, aging, leaderboard) run locally on your synced data, so they are not gated behind a reporting add-on.

### Will this burn through my Pipedrive API rate limits?

Day-to-day questions answer from the local mirror after a `sync`, so they make zero API calls. `sync` itself paginates politely - tune `--rate-limit` and `--concurrency`, and use `--since 24h` to refresh only what changed - and live calls only happen when local data is missing or stale.


## More CRM connectors

Run more than one CRM tool, or comparing options? These connectors work the same way: [HubSpot](/skills/hubspot/) · [Salesbuildr](/skills/salesbuildr/)

## Status

Beta. Validated against the Pipedrive API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
