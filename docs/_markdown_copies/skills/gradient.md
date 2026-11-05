> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/gradient/

# The Gradient MSP MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Gradient MSP, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Gradient MSP. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Gradient MSP to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI to push tonight's usage counts, tell you which accounts' billing changed since the last push, and confirm an alert actually became a PSA ticket - and it runs the real Gradient MSP Synthesize commands and shows you the receipts. The full vendor API from your terminal, plus a local push ledger and offline mirror that Gradient's PowerShell SDK cannot give you.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/gradient/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/gradient)

## Instead of clicking through Gradient MSP, just ask

**Instead of** Write a PowerShell script that loops the count endpoint once per account and hope you didn't kick off a billing rebuild on every single call
**just ask:** *"Push tonight's usage counts from this file in one shot"*
Your agent runs: `gradient-cli usage push --file ./counts.csv --agent`

**Instead of** Export the last two pushes to spreadsheets and eyeball which accounts moved before you invoice
**just ask:** *"Show me which accounts' usage changed since my last push"*
Your agent runs: `gradient-cli usage drift --agent`

**Instead of** Fire an alert into Synthesize and click into the PSA later to see whether a ticket ever showed up
**just ask:** *"Send this alert and tell me when the PSA ticket actually exists"*
Your agent runs: `gradient-cli alert send --account "123456789" --title "Backup failure" --wait --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/gradient/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Push a whole file of usage counts and rebuild billing exactly once? | `gradient-cli usage push --file ./counts.csv --agent` |
| Which accounts' usage changed between my last two pushes? | `gradient-cli usage drift --agent` |
| Send an alert and confirm the PSA ticket was actually created? | `gradient-cli alert send --account "123456789" --title "Backup failure" --wait --agent` |
| Which of my dispatched alerts never became tickets? | `gradient-cli alert trace --stuck --agent` |
| Which accounts are unmapped or missing a vendor SKU? | `gradient-cli hygiene unmapped --agent` |
| Is my integration ready to flip to active? | `gradient-cli status ready --agent` |
| Add a single ad-hoc unit count for one account and service? | `gradient-cli billing <serviceId> --account-id "123456789" --unit-count 42` |
| Are my credentials valid and what's my integration status? | `gradient-cli integration get --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/gradient/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/gradient/guide.md).

## What makes this one different

Most ways to script the Synthesize API proxy each question straight to a live call and remember nothing, so 'what changed since last night?' is unanswerable without exporting and diffing spreadsheets yourself. This CLI keeps a local push ledger and an offline SQLite mirror of your accounts, so usage drift, mapping-hygiene rollups, and stuck-alert traces are one command instead of a data-export project. Bulk pushes defer the billing rebuild to a single call at the end rather than firing one per row.

Gradient's Synthesize portal is where MSPs review and approve mappings; it has no terminal, no bulk-push-with-one-rebuild primitive, and no scriptable alert-to-ticket confirmation. This CLI complements the portal: it is the agent-native way to feed and audit the data the portal reconciles, not a replacement for the approval workflow.

## The pain this closes

- Billing reconciliation is a monthly fire drill: pull a CSV from each vendor, hand-map the fields to your PSA, and hope you caught every seat that drifted. r/msp threads return to it every cycle, and Gradient's own data puts the manual version at up to 90% more time than automating it.
- Synthesize ships a PowerShell SDK that wants a script project per integration, so usage pushes end up living in brittle one-off scripts with no record of what was sent or what changed between runs.
- Fire-and-forget alerting means you never know whether an alert became a PSA ticket until someone goes looking, and a count push with no audit trail makes 'why did this invoice change?' unanswerable.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/gradient/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/gradient/install.ps1 | iex
```

After install, authenticate once with your Gradient MSP credentials, then verify with `gradient-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | integration get, vendor get, services get, accounts list, usage drift, alert trace, hygiene unmapped, status ready, analytics | Allow |
| Write (routine) | usage push, billing, alert send, alerting, import, accounts create, accounts update, accounts update-one, services create, mappings create, mappings update, mappings update-bulk, integration update-status, vendor update | Preview with --dry-run, then a reviewed write |
| Destructive / config | auth set-token and auth logout (local credential file only); the Synthesize vendor API exposes no delete commands | Human-in-the-loop only |

The skill reads your Synthesize accounts, services, mappings, integration status, and local push ledger, and it can write: push usage counts, create or update accounts, services, and mappings, dispatch alerts, and flip integration status. There are no vendor-side delete commands. Keep an autonomous agent to reads plus previewed (--dry-run) writes, and require a human to approve any count push, mapping change, or status flip. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/gradient/governance.md).

## Frequently asked questions

### Is there an MCP server for Gradient MSP?

Yes - this one. A free, open source MCP server and Claude Code Skill for Gradient MSP, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Gradient MSP MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `GRADIENT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `GRADIENT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way gradient-mcp runs, and shares MCP configuration with Codex: add gradient-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Gradient MSP data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Which credentials do I need, and how do I set them?

A Synthesize vendor token. Set GRADIENT_TOKEN to the base64 of <vendorApiKey>:<partnerApiKey>, or set GRADIENT_VENDOR_API_KEY and GRADIENT_PARTNER_API_KEY and the CLI derives the token for you. Run gradient-cli integration get to confirm the credential works.

### Does this replace the Synthesize portal or Managed Billing Reconciliation?

No. Mapping approval and reconciliation review still happen in the Synthesize portal (or are handled by Gradient's MBR service). This CLI is the vendor and integration side: it pushes accounts, services, and usage counts in, audits what was pushed, and traces alerts to PSA tickets. It complements the portal, it does not log into it.

## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](https://msp-skills.compoundingteams.com/skills/appdirect.md) · [AWS](https://msp-skills.compoundingteams.com/skills/aws-billing.md) · [DataGate](https://msp-skills.compoundingteams.com/skills/datagate.md) · [Maxio](https://msp-skills.compoundingteams.com/skills/maxio.md) · [Pax8](https://msp-skills.compoundingteams.com/skills/pax8.md) · [QuickBooks Online](https://msp-skills.compoundingteams.com/skills/quickbooks.md) · [Sherweb](https://msp-skills.compoundingteams.com/skills/sherweb.md) · [TD SYNNEX StreamOne ION](https://msp-skills.compoundingteams.com/skills/td-synnex.md) · [Xero](https://msp-skills.compoundingteams.com/skills/xero.md)

## Status

Beta. Validated against the Gradient MSP API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
