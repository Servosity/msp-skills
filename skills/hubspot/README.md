# HubSpot + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the HubSpot
> API. Not affiliated with, endorsed by, or sponsored by HubSpot Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/hubspot/">
    <img src="../../docs/assets/video/hubspot/animated-og.gif" alt="HubSpot demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/hubspot/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every HubSpot Sales Hub feature, plus offline cross-object queries, property-change-history reporting, and an agent-native data layer no other HubSpot tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the HubSpot skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `hubspot-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `hubspot-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `hubspot-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the HubSpot MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `hubspot-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the HubSpot Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.ps1 | iex`. Then authenticate per the README and run `hubspot-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hubspot/install.sh)
```

The installer drops both `hubspot-cli` (the CLI) and `hubspot-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
hubspot-cli --version
```

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/hubspot --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/hubspot --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `hubspot-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the hubspot skill from https://github.com/servosity/msp-skills/tree/main/skills/hubspot. The skill defines how its required CLI (`hubspot-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your HubSpot portal):

```bash
HUBSPOT_ACCESS_TOKEN=<value> hubspot-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which meetings were *ever* in outcome "Scheduled" this month  -  even the ones that later flipped to No Show or Completed? | `hubspot-cli meetings status-report --status scheduled --month 2026-04` |
| Who do I call today? (the daily nurture list) | `hubspot-cli nurture queue --owner me --top 20 --agent` |
| Which contacts or deals have gone cold? | `hubspot-cli stale contacts --days 30 --owner me` |
| What's my pipeline health right now? ($ at risk + oldest stuck deals) | <!-- cli-claims:ignore -->`hubspot-cli pipeline-health default --idle-days 14` |
| Are my reps overloaded? | `hubspot-cli owner-load --pipeline default` |
| What's the cross-object timeline for this deal? (every call + email + meeting + note + task) | `hubspot-cli engagements of deal:1234567 --since 90d` |
| What changed across contacts, deals, engagements, and companies since yesterday? | `hubspot-cli since 24h` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most HubSpot integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "every deal that was at stage X *at some point* last quarter  -  across all 200 active accounts  -  with $ at risk weighted by stage probability and current rep load." The standard HubSpot search/filter API physically cannot answer the "was at stage X at some point" half of that question  -  only the property-history endpoint can  -  and chaining a join across deals, contacts, owners, engagements, and property snapshots through live REST is a 50-call dance the AI burns context on.

This skill syncs HubSpot into a **local SQLite mirror** with full-text search **plus a property-history snapshot table** (the only HubSpot CLI/MCP that captures `propertiesWithHistory` into a queryable shape). Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `meetings status-report`, `pipeline-health`, and `engagements of` join across **deals × contacts × owners × engagements × per-property history snapshots**  -  work a stateless API wrapper can't do.

## The pain this closes

**The pain:** monthly customer reports. An MSP customer asks "how many meetings were Scheduled with us in April?"  -  but by report time, every one of those meetings has flipped to Completed or No Show. HubSpot's UI can't answer "was *ever* Scheduled"; the standard search/filter API can't either; the property-history endpoint can, but no public HubSpot CLI or MCP exposes it in a queryable shape. See the [HubSpot Community thread on V3 property-history](https://community.hubspot.com/t5/APIs-Integrations/Enable-retrieval-of-property-history-in-the-V3-APIs/m-p/496344) for the long-running operator request, and the property-history retention discussion on [r/hubspot](https://www.reddit.com/r/hubspot/) for the workflow-impact framing.

This skill closes that gap with five commands an MSP owner can run from a single AI prompt:

- `hubspot-cli sync --resources hubspot-meetings-crm --with-history hs_meeting_outcome,hs_meeting_title,hubspot_owner_id`  -  captures the property snapshots so the report has data
- `hubspot-cli meetings status-report --status scheduled --month 2026-04 --csv`  -  the customer report itself, one command
- `hubspot-cli meetings ever-had --property hs_meeting_outcome --value Scheduled --from 2026-04-01 --to 2026-04-30`  -  same data, ad-hoc query
- <!-- cli-claims:ignore -->`hubspot-cli pipeline-health default --idle-days 14`  -  the rest of the QBR: what's at risk this period
- `hubspot-cli engagements of deal:<id> --since 90d`  -  full activity trail for any specific deal under discussion

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **paid ChatGPT plans** - ChatGPT's MCP connector support is in beta and plan-dependent, so check [OpenAI's current guidance](https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt-beta) for which tiers expose it. ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The HubSpot MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your HubSpot credentials once.

### Is my HubSpot data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### How is this different from HubSpot's own Agent CLI (announced May 2026)?

HubSpot announced their own [Agent CLI](https://blog.hubspot.com/marketing/introducing-the-hubspot-agent-cli) on 2026-05-27  -  a stateless, live-API CLI for scheduled GTM automations (CRUD, JSONL, safety-gated bulk ops). This skill is a **stateful, offline-first companion**: it does what HubSpot's CLI doesn't  -  local SQLite mirror, full-text search, cross-object joins in one command, and the property-history wedge (`meetings ever-had`, `meetings status-report`) that HubSpot's CLI doesn't expose. Idiom-compatible where it matters (`--filter` grammar, JSONL stdin/out, and the `contacts bulk-update` dry-run → digest → confirm dance for large batches), so prompts written for HubSpot's CLI mostly port over.

### Will this hit my HubSpot API rate limits?

The whole point of the local mirror is to **stop hitting the API for read queries**. After the first `sync` (which is paged and rate-limit-aware - `--with-history` requests `propertiesWithHistory`, which per [HubSpot's batch-read API docs](https://developers.hubspot.com/docs/api/crm/understanding-the-crm) caps the records returned per request, so the CLI pages accordingly), every aggregate report runs against your local SQLite  -  zero API calls. Sync itself is gentle: pagination is built-in, you can scope with `--resources` and `--since 7d`, and the CLI emits sync-warning events when HubSpot throttles instead of swallowing them silently.

### Do I need to be a HubSpot partner or pay for a specific tier?

No partnership required. You need a HubSpot **Private App access token** (free to create from any portal at [app.hubspot.com/private-apps](https://app.hubspot.com/private-apps)) with read scopes for the objects you care about (contacts, deals, meetings, etc.). Property-history retention varies by tier  -  HubSpot keeps ~90 days on free/Starter and longer on paid tiers; the CLI captures whatever's in the response and accrues forward from your first `--with-history` sync.

### Will this replace my HubSpot UI?

No. The HubSpot UI is great for one record at a time, deal editing, drag-and-drop pipelines, and the things your reps live in daily. This skill is for the **aggregate questions** your reps and AI agent can't answer fast from the UI: "what changed across 200 deals this week," "who's at risk this quarter," "every meeting that was Scheduled in April even if it later flipped." Different surface, same data.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

Read the honest version first: **writes are not gated by default.** `--dry-run` is an opt-in global flag ("Show request without sending", default off) - a raw create/update/delete command sends immediately on first run unless your agent passes `--dry-run` first. The one built-in mutation gate lives on `contacts bulk-update`, and it only fires **above 100 rows** (smaller batches dispatch immediately with a one-line bypass warning). So the real control is an **agent-level policy** you set, plus the scope you grant the token.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | All reports, rollups, and search (`stale`, `pipeline-health`, `nurture queue`, `deals top`, `engagements of`, `meetings history`, `meetings ever-had`, `meetings status-report`, `since`, `notes signals`, `owner-load`, `nurture-mine`, `history`) | Allow |
| **Write (routine)** | `contacts bulk-update` (digest + `--confirm` gate, but only above 100 rows - smaller batches send immediately), `crm post-…-batch-create/update/upsert` (88 total; these raw writes have no `--confirm` flag and send immediately) | Make the agent pass `--dry-run` to preview, then require human approval of the exact command before sending. Do not rely on a built-in gate for small batches. |
| **Credential / security** | (none detected) | Human-in-the-loop only |
| **Destructive** | `crm delete-…-archive`, `*-crm delete-…`, `hubspot-contacts-crm post-…-gdpr-delete` (24 total) | Human-in-the-loop only, explicit confirmation |
| **Admin** | (none detected) | Operator-only, not for agents |

The strongest control is the **scope you grant the HubSpot credentials** - the CLI can only do what the credentials are permitted to do. A read/report workflow should use a read-only token. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the HubSpot API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-04._
