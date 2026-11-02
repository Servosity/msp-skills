> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/quickbooks/

# The QuickBooks Online MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Intuit Inc..

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-06-15. The version it verified was not recorded.

Yes - there is an MCP server for QuickBooks Online. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects QuickBooks Online to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your books a question and get the answer, not a CSV export. The skill syncs your QuickBooks Online company into a local SQLite mirror, then answers who owes you (ar-aging), what you owe (ap-aging), where the cash is (balances), which invoices to chase first (invoices stale), and whether the books are clean to close (reconcile) - instantly, offline, across the whole book. No portal clicking, no per-question API call.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/quickbooks/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/quickbooks)

## Instead of clicking through QuickBooks Online, just ask

**Instead of** Exporting the A/R Aging Summary to CSV and pivoting it in Excel to see who is overdue
**just ask:** *"Who owes us money, and how overdue are they?"*
Your agent runs: `quickbooks-cli ar-aging --agent`

**Instead of** Clicking through every open invoice to build this week's collections call list by hand
**just ask:** *"Which overdue invoices should I chase first?"*
Your agent runs: `quickbooks-cli invoices stale --days 30 --agent`

**Instead of** Eyeballing the bank balance and guessing whether next month's bills will clear
**just ask:** *"What does our cash position look like over the next month?"*
Your agent runs: `quickbooks-cli cash-forecast --weeks 4 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/quickbooks/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who owes us money, bucketed 0-30 / 31-60 / 61-90 / 90+? | `quickbooks-cli ar-aging --agent` |
| What do we owe vendors, and when is it due? | `quickbooks-cli ap-aging --agent` |
| Which overdue invoices should I chase first? | `quickbooks-cli invoices stale --days 30 --agent` |
| Where does our cash stand across accounts, AR, and AP? | `quickbooks-cli balances --agent` |
| What net cash movement is scheduled over the next 4 weeks? | `quickbooks-cli cash-forecast --weeks 4 --agent` |
| What is our DSO, and who are the slowest payers? | `quickbooks-cli dso --agent` |
| Are the books clean enough to close this month? | `quickbooks-cli reconcile --agent` |
| Which payments came in but were never applied to an invoice? | `quickbooks-cli payments unapplied --agent` |
| Which customers are duplicated in our list? | `quickbooks-cli dupes customers --agent` |
| Who slipped an aging bucket since our last check? | `quickbooks-cli aging-delta --agent` |
| What were our recognized revenue, gross margin, and net income last month? | `quickbooks-cli report pnl --agent` |
| What is our deferred revenue balance right now? | `quickbooks-cli report balance-sheet --highlight "deferred revenue" --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/quickbooks/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/quickbooks/guide.md).

## What makes this one different

Most QuickBooks integrations and MCP servers proxy each question into a live API call - fine for fetching one invoice, useless for "who is overdue across the whole book" because QuickBooks has no single endpoint that returns aging, DSO, or a reconciliation verdict. This skill syncs your company into a local SQLite mirror once, then computes those answers offline with real date math and cross-entity joins (invoices to payments to customers) - instant, no rate-limit pressure, and the agent sees the answer, not raw JSON pages.

QuickBooks Online's own reports are point-in-time and live in the portal - you click, export, and pivot. This skill answers the same questions from your terminal or AI agent, adds cross-run memory (aging-delta) the portal does not keep, and composes a one-shot month-end reconcile sweep the UI makes you assemble by hand. It complements your books of record; it does not replace them.

## The pain this closes

- Month-end close eats a day: chasing unapplied payments, deduping near-identical customer records, and proving the books actually balance before they go to the accountant - the kind of bookkeeping overhead MSP owners vent about on r/msp and r/QuickBooks.
- Receivables age silently. By the time you pull the A/R aging report the 90+ bucket already holds money you may never collect, and QuickBooks reports are point-in-time - so you cannot see who slipped a bucket since last month without keeping your own spreadsheet history.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quickbooks/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex
```

After install, authenticate once with your QuickBooks Online credentials, then verify with `quickbooks-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | ar-aging, ap-aging, balances, dso, cash-forecast, reconcile, dupes, search, report pnl, report balance-sheet, plus every list and get | Allow |
| Write (routine) | invoices / bills / payments / customers / vendors / accounts / items / journal-entries create and update (16 commands), plus bulk import | Preview with --dry-run, then a reviewed write |
| Destructive / config | invoices delete, bills delete, payments delete, journal-entries delete (hard deletes) | Human-in-the-loop only |

The skill reads everything - aging, balances, DSO, cash forecast, reconciliation, search - and read commands cannot change anything. Writes are explicit create/update/delete commands on invoices, bills, payments, customers, vendors, accounts, items, and journal entries, and `--dry-run` is opt-in, so the recommended agent policy is: read freely, preview every write, and keep a human on deletes. Credentials are read from the environment only and are never written to disk or logged. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/quickbooks/governance.md).

## Frequently asked questions

### Is there an MCP server for QuickBooks Online?

Yes - this one. A free, open source MCP server and Claude Code Skill for QuickBooks Online, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the QuickBooks Online MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `QUICKBOOKS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `QUICKBOOKS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running quickbooks-mcp in HTTP mode, and in this build HTTP mode asks for no password, listens on every network interface, and can be reached by a web page you open. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my QuickBooks Online data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my QuickBooks API rate limits?

Rarely. The only API-heavy step is the one-time sync that mirrors your company into local SQLite; after that, ar-aging, dso, cash-forecast, reconcile and the rest run entirely offline against the mirror. Intuit throttles per company (realm), and the CLI paginates and rate-limits sync for you, so day-to-day questions never touch the API.

### Do I need to be an Intuit partner to use it?

No. You need a QuickBooks Online company and an OAuth access token scoped to com.intuit.quickbooks.accounting, plus your company realm ID. You mint the token from the Intuit Developer portal or the OAuth 2.0 Playground, and `quickbooks-cli auth refresh` turns a refresh token into a fresh access token - no partner status required.

## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](https://msp-skills.compoundingteams.com/skills/appdirect.md) · [AWS](https://msp-skills.compoundingteams.com/skills/aws-billing.md) · [DataGate](https://msp-skills.compoundingteams.com/skills/datagate.md) · [Gradient MSP](https://msp-skills.compoundingteams.com/skills/gradient.md) · [Maxio](https://msp-skills.compoundingteams.com/skills/maxio.md) · [Pax8](https://msp-skills.compoundingteams.com/skills/pax8.md) · [Sherweb](https://msp-skills.compoundingteams.com/skills/sherweb.md) · [TD SYNNEX StreamOne ION](https://msp-skills.compoundingteams.com/skills/td-synnex.md) · [Xero](https://msp-skills.compoundingteams.com/skills/xero.md)

## Status

Live-verified. Confirmed by Servosity (maintainer) against their own production tenant. The version it verified was not recorded. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
