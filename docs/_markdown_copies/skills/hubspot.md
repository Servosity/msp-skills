> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/hubspot/

# The HubSpot MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by HubSpot, Inc..

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-06-05. The version it verified was not recorded.

Yes - there is an MCP server for HubSpot. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects HubSpot to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs run HubSpot as the sales CRM - pipeline, deals, quote-chasing. Ask your AI "which deals went cold," "what's my pipeline health," or "who do I call today," and get an answer the portal can't compose: cross-object rollups across deals, contacts, owners, and engagements, computed offline in one query instead of a dozen exports and saved views.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/hubspot/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/hubspot)

## Instead of clicking through HubSpot, just ask

**Instead of** Filtering the deals board, eyeballing last-activity dates, and guessing which open deals have stalled
**just ask:** *"Which of my open deals have gone cold with no activity in three weeks?"*
Your agent runs: `hubspot-cli stale deals --days 21 --owner me`

**Instead of** Building a saved view per stage, exporting, and summing $ and counts by hand for the pipeline review
**just ask:** *"What's my pipeline health right now - count, dollars, and what's at risk per stage?"*
Your agent runs: `hubspot-cli pipeline-health --idle-days 14`

**Instead of** Pulling per-rep deal lists one at a time to see who is overloaded before the Monday sales meeting
**just ask:** *"How are open deals spread across my reps by stage and dollar value?"*
Your agent runs: `hubspot-cli owner-load --pipeline default`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/hubspot/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

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
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

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

### Is there an MCP server for HubSpot?

Yes - this one. A free, open source MCP server and Claude Code Skill for HubSpot, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the HubSpot MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `HUBSPOT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `HUBSPOT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way hubspot-mcp runs, and shares MCP configuration with Codex: add hubspot-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

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

## More CRM connectors

Run more than one CRM tool, or comparing options? These connectors work the same way: [Pipedrive](https://msp-skills.compoundingteams.com/skills/pipedrive.md) · [QuoteWerks](https://msp-skills.compoundingteams.com/skills/quotewerks.md) · [Salesbuildr](https://msp-skills.compoundingteams.com/skills/salesbuildr.md)

## Status

Live-verified. Confirmed by Servosity (maintainer) against their own production tenant. The version it verified was not recorded. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
