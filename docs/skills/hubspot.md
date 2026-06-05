---
layout: default
title: "HubSpot MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every HubSpot Sales Hub feature, plus offline cross-object queries, property-change-history reporting, and an agent-native data layer no other HubSpot tool has."
permalink: /skills/hubspot/
skill_name: "HubSpot MCP"
image: /assets/social/hubspot/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local HubSpot MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my HubSpot data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my HubSpot API rate limits?"
    a: "The local mirror exists so you stop hitting the API for reads. After the first sync (which respects HubSpot's pagination), every aggregate report runs against your local SQLite with zero API calls. You can scope sync with --resources and --since to keep it light, and the CLI surfaces sync warnings rather than swallowing them."
  - q: "Do I need to be a HubSpot partner or a paid tier?"
    a: "No partnership is required. You create a HubSpot Private App access token from your portal with read scopes for the objects you care about. Property-history retention varies by HubSpot plan, so the depth of historical reporting depends on your tier; the CLI captures whatever the API returns and accrues forward from your first --with-history sync."
  - q: "Does it work with HubSpot's free CRM?"
    a: "It works against any portal that issues a Private App access token with the read scopes you grant. Which objects, properties, and history depth are available is governed by your HubSpot plan and the scopes on the token, not by this skill."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your HubSpot credentials once; hubspot-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a HubSpot question in plain language; it runs hubspot-cli for you."
---

# HubSpot + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the HubSpot
> API. Not affiliated with, endorsed by, or sponsored by HubSpot, Inc..

**Live-verified** - confirmed by a real MSP against a live HubSpot tenant (2026-06-05).

MSPs run HubSpot as the sales CRM - pipeline, deals, quote-chasing. Ask your AI "which deals went cold," "what's my pipeline health," or "who do I call today," and get an answer the portal can't compose: cross-object rollups across deals, contacts, owners, and engagements, computed offline in one query instead of a dozen exports and saved views.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/hubspot){: .btn}

## Instead of clicking through HubSpot, just ask

**Instead of** Filtering the deals board, eyeballing last-activity dates, and guessing which open deals have stalled
**just ask:** *"Which of my open deals have gone cold with no activity in three weeks?"*
<sub>Your agent runs: <code>hubspot-cli stale deals --days 21 --owner me</code></sub>

**Instead of** Building a saved view per stage, exporting, and summing $ and counts by hand for the pipeline review
**just ask:** *"What's my pipeline health right now - count, dollars, and what's at risk per stage?"*
<sub>Your agent runs: <code>hubspot-cli pipeline-health --idle-days 14</code></sub>

**Instead of** Pulling per-rep deal lists one at a time to see who is overloaded before the Monday sales meeting
**just ask:** *"How are open deals spread across my reps by stage and dollar value?"*
<sub>Your agent runs: <code>hubspot-cli owner-load --pipeline default</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/hubspot/wide-1200x630.png" src="/assets/video/hubspot/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/hubspot/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which open deals have gone cold with no engagement in the last three weeks? | `hubspot-cli stale deals --days 21 --owner me` |
| Which of my contacts haven't been touched in a month? | `hubspot-cli stale contacts --days 30 --owner me` |
| What's my pipeline health right now - per-stage count, dollars, and what's at risk? | `hubspot-cli pipeline-health --idle-days 14` |
| How is the open-deal load spread across my reps? | `hubspot-cli owner-load --pipeline default` |
| Who should I call today, ranked by stale-days, deal size, and stage? | `hubspot-cli nurture queue --owner me --top 20` |
| Which are my top deals by composite score (signal, amount, stage, recency)? | `hubspot-cli deals top --top 5 --owner me` |
| What's the full activity trail for a specific deal - every call, email, meeting, note, and task? | `hubspot-cli engagements of deal:456 --since 90d` |
| Which meetings were ever Scheduled in a given month, even after they flipped to Completed or No Show? | `hubspot-cli meetings status-report --status scheduled --month 2026-04` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/hubspot/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/hubspot/guide.md).

## What makes this one different

Most HubSpot integrations and MCP servers proxy each question into a live API call - fine for one record, but a stalls-across-200-accounts question becomes a multi-call dance the AI burns context on, and the standard search API physically cannot answer "was ever in stage X." This skill syncs HubSpot into a local SQLite mirror with full-text search plus a property-history snapshot table, so aggregate questions become one local join: instant, offline, and the AI sees the answer, not the raw data.

It complements HubSpot's own AI and Agent CLI rather than replacing them: the portal stays best for one-record editing and drag-and-drop pipelines, while this skill answers the cross-object, cross-client, was-ever-in-state-X questions that live-API tools can't compose in a single query.

## The pain this closes

- Monthly customer reports lose data the moment a status changes: "how many meetings were Scheduled with this client in April?" can't be answered once those meetings flip to Completed or No Show, because the portal and the standard search API only filter on current values (HubSpot Community: "Enable retrieval of property history in the V3 APIs").
- The property-history endpoint can answer "was ever in state X," but no public HubSpot CLI or MCP exposes it in a queryable shape - recurring r/hubspot threads ask for historical state for monthly reporting.
- HubSpot's own Agent CLI (announced 2026-05-27) is stateless and live-API; its public skills repo has zero references to propertiesWithHistory, so the history-reporting gap stays open.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hubspot/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hubspot/install.ps1 | iex
```

After install, authenticate once with your HubSpot credentials, then verify with `hubspot-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | hubspot-cli stale deals --days 21 --owner me; hubspot-cli pipeline-health --idle-days 14; hubspot-cli owner-load --pipeline default; hubspot-cli nurture queue --owner me --top 20; hubspot-cli engagements of deal:456 --since 90d; hubspot-cli since 24h | Allow |
| Write (routine) | hubspot-cli contacts bulk-update (digest+--confirm gate only above 100 rows; smaller batches send immediately with a bypass warning); hubspot-cli batch post-crm-v3-objects-object-type-create-create; hubspot-cli batch post-crm-v3-objects-object-type-update-update (raw writes send immediately - no --confirm flag) | Pass --dry-run first to preview; raw writes are NOT gated by default, so require agent-side approval before sending |
| Destructive / config | hubspot-cli hubspot-deals-crm delete-v3-objects-0-3-deal-id-archive; hubspot-cli hubspot-contacts-crm delete-v3-objects-contacts-contact-id; hubspot-cli hubspot-contacts-crm post-v3-objects-contacts-gdpr-delete | Human-in-the-loop only |

The skill drives the hubspot-cli and hubspot-mcp binaries, authenticating only with HUBSPOT_ACCESS_TOKEN read from the environment - never written to disk, logged, or sent anywhere except the HubSpot API. Read commands (reports, rollups, search) are always safe and can change nothing. Writes are not gated by default: --dry-run is an opt-in flag (default off) that previews a request without sending, so raw create/update/delete commands send immediately on first run unless your agent passes --dry-run first. The one built-in mutation gate is on contacts bulk-update, and it only fires above 100 rows (smaller batches dispatch immediately with a one-line bypass warning). The recommended policy is an agent-level rule: require the agent to show the exact command and get human approval before any mutation, and require a human for any destructive or credential-touching command. The strongest control is the scope you grant the Private App token. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/hubspot/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local HubSpot MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my HubSpot data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my HubSpot API rate limits?

The local mirror exists so you stop hitting the API for reads. After the first sync (which respects HubSpot's pagination), every aggregate report runs against your local SQLite with zero API calls. You can scope sync with --resources and --since to keep it light, and the CLI surfaces sync warnings rather than swallowing them.

### Do I need to be a HubSpot partner or a paid tier?

No partnership is required. You create a HubSpot Private App access token from your portal with read scopes for the objects you care about. Property-history retention varies by HubSpot plan, so the depth of historical reporting depends on your tier; the CLI captures whatever the API returns and accrues forward from your first --with-history sync.

### Does it work with HubSpot's free CRM?

It works against any portal that issues a Private App access token with the read scopes you grant. Which objects, properties, and history depth are available is governed by your HubSpot plan and the scopes on the token, not by this skill.


## Status

Beta. Validated against the HubSpot API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
