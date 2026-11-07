> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/maxio/

# The Maxio MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Maxio LLC.

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-11-06.

Yes - there is an MCP server for Maxio. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Maxio to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask 'what's our MRR, and how did it move last quarter?' and get the New / Expansion / Contraction / Churn / Reactivation waterfall, net and gross revenue retention, per-customer revenue history, and a ranked list of accounts that need attention - in one command, computed offline from a local mirror of your Maxio Advanced Billing data. It is the revenue math the dashboard buries and the live, point-in-time API can't reconstruct after the fact.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/maxio/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/maxio)

## Instead of clicking through Maxio, just ask

**Instead of** Exporting subscription CSVs into a spreadsheet every month to rebuild the MRR movement waterfall by hand
**just ask:** *"Show me the MRR waterfall by month since January"*
Your agent runs: `maxio-cli mrr waterfall --since 2026-01-01 --group-by month`

**Instead of** Clicking through the portal account by account to spot who is contracting, churning, or up for a big renewal
**just ask:** *"Which accounts need my attention this week?"*
Your agent runs: `maxio-cli triage --limit 20`

**Instead of** Pulling Maxio data into a separate BI tool just to compute net revenue retention
**just ask:** *"What's our net and gross revenue retention this year?"*
Your agent runs: `maxio-cli retention --since 2026-01-01 --group-by month`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/maxio/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's our current MRR and ARR right now? | `maxio-cli mrr now` |
| How did MRR move month over month (new, expansion, contraction, churn, reactivation)? | `maxio-cli mrr waterfall --since 2026-01-01 --group-by month` |
| What's our net and gross revenue retention, and quick ratio? | `maxio-cli retention --since 2026-01-01 --group-by month` |
| What's recurring revenue for this customer, and how has it trended? | `maxio-cli mrr client --customer "Acme"` |
| Which accounts need attention this week? | `maxio-cli triage --limit 20` |
| How much MRR is up for renewal in the next 30 days? | `maxio-cli renewals --within 30` |
| Which usage components drove expansion versus contraction? | `maxio-cli usage-drivers --since 2026-01-01` |
| Where does normalized MRR diverge from what we actually invoiced? | `maxio-cli reconcile --since 2026-01-01` |
| How many new logos did we land this quarter, and how much MRR? | `maxio-cli new-customers --since 3m` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/maxio/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/maxio/guide.md).

## What makes this one different

Most Maxio integrations proxy each question into a live API call - fine for reading one record, useless for trending, because the live MRR endpoints return only point-in-time figures (and require Maxio's Insights/Analytics add-on). This skill syncs Maxio into a local SQLite mirror and snapshots each sync into a time series, so the waterfall, retention curves, and cohort history are computed locally as a trend the live, point-in-time API can't reconstruct after the fact.

The Maxio dashboard shows point-in-time numbers behind clicks and exports; this skill answers the trended, cross-object revenue questions in one command from the terminal or your AI agent, and keeps the history on your own machine rather than depending on a live API that returns only point-in-time numbers. It complements the portal, which still owns billing operations and configuration.

## The pain this closes

- Maxio's recurring-revenue reporting - MRR, churn, ARPA, retention - is thin enough that operators routinely export the data into a separate BI tool to answer board questions (a recurring theme in Maxio's G2 reviews).
- Simple asks like 'list the customers who signed up this quarter and the revenue they brought' have no one-click answer in the portal, and the live API returns only point-in-time figures, with no endpoint to reconstruct the historic trend.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/maxio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/maxio/install.ps1 | iex
```

After install, authenticate once with your Maxio credentials, then verify with `maxio-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | mrr now, mrr waterfall, retention, cohort, triage, reconcile, renewals, churn, new-customers, search | Allow |
| Write (routine) | subscriptions-json create-subscription, customers update, components update | Preview with --dry-run, then a reviewed write |
| Destructive / config | customers delete, subscription-groups delete, payment-profiles delete-unused, reason-codes delete | Human-in-the-loop only |

Read commands - every MRR, retention, cohort, triage, and reconcile rollup plus search - cannot change anything and are safe to run unattended. Routine writes (creating or updating subscriptions, components, or customers) send immediately unless you pass --dry-run first, so the recommended agent policy is preview-then-approve. Credential, destructive (deletes and cancellations), and admin commands should always be human-in-the-loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/maxio/governance.md).

## Frequently asked questions

### Is there an MCP server for Maxio?

Yes - this one. A free, open source MCP server and Claude Code Skill for Maxio, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Maxio MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `MAXIO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `MAXIO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way maxio-mcp runs, and shares MCP configuration with Codex: add maxio-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Maxio data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Maxio API rate limits?

Rarely, for the analytics. After you sync, the revenue-analytics commands (mrr waterfall, retention, cohort, triage, reconcile) read from the local SQLite mirror, so they make zero API calls. Keeping the mirror current is two incremental commands - 'maxio-cli sync' for the full dataset and 'maxio-cli mrr sync' to snapshot MRR-movement history - and both fetch only what changed. The endpoint-mirror commands and tail read live from Maxio, so those count against your limits.

### Do I need to be a Maxio partner or customer to use this?

You need your own Maxio Advanced Billing API credentials (a username and password) - that's it. This is an unofficial, community-built skill, not a Maxio product, so there is no partner program or separate signup. It reads only the data your credentials already grant.

## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](https://msp-skills.compoundingteams.com/skills/appdirect.md) · [AWS](https://msp-skills.compoundingteams.com/skills/aws-billing.md) · [DataGate](https://msp-skills.compoundingteams.com/skills/datagate.md) · [Gradient MSP](https://msp-skills.compoundingteams.com/skills/gradient.md) · [Pax8](https://msp-skills.compoundingteams.com/skills/pax8.md) · [QuickBooks Online](https://msp-skills.compoundingteams.com/skills/quickbooks.md) · [Sherweb](https://msp-skills.compoundingteams.com/skills/sherweb.md) · [TD SYNNEX StreamOne ION](https://msp-skills.compoundingteams.com/skills/td-synnex.md) · [Xero](https://msp-skills.compoundingteams.com/skills/xero.md)

## Status

Live-verified. Confirmed by Servosity (maintainer) against their own production tenant. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
