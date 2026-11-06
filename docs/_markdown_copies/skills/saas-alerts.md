> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/saas-alerts/

# The SaaS Alerts MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for SaaS Alerts. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects SaaS Alerts to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running SaaS Alerts across a book of customers? Ask your AI "what fired overnight, critical first," "which Respond triggers expire next," or "is one IP hitting several clients" and get one answer instead of paging customer by customer. One partner key reaches all four SaaS Alerts APIs, including Respond, and contain and rule-triggers decide preview every change until you confirm it.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/saas-alerts/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/saas-alerts)

## Instead of clicking through SaaS Alerts, just ask

**Instead of** Opening the SaaS Alerts dashboard each morning and paging through events customer by customer to work out what fired overnight and which accounts it hit
**just ask:** *"What fired across all my customers since yesterday, critical first, grouped by customer and user?"*
Your agent runs: `saas-alerts-cli triage --since 24h --min-severity medium`

**Instead of** Watching the Respond queue so manual-approval triggers are decided before they lapse (they auto-expire after 12 hours or when the rule fires again)
**just ask:** *"Which Respond triggers expire in the next two hours, and what would approving each one do?"*
Your agent runs: `saas-alerts-cli rule-triggers queue --expiring-within 2h`

**Instead of** Clicking through block sign-in, expire sessions and reset password one at a time for a compromised Microsoft 365 user
**just ask:** *"Show me exactly what it would take to lock down jane.doe@example.com: block sign-in, kill sessions, reset the password."*
Your agent runs: `saas-alerts-cli contain "jane.doe@example.com" --actions block,expire,reset`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/saas-alerts/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What fired across every customer in the last 24 hours, critical first? | `saas-alerts-cli triage --since 24h --min-severity medium` |
| Which Respond triggers expire or auto-ignore soonest? | `saas-alerts-cli rule-triggers queue --expiring-within 2h` |
| Is one IP address hitting two or more of my customers? | `saas-alerts-cli cross-tenant --by ip --since 24h --min-customers 2` |
| Everything on this one account: events, country hops, Respond triggers, devices | `saas-alerts-cli investigate "jane.doe@example.com" --since 7d` |
| Which customers went silent, lost their Respond connection or have no Respond rules? | `saas-alerts-cli coverage --silent 72h` |
| Why did SaaS Alerts tickets stop arriving? Is a webhook disabled? | `saas-alerts-cli subscriptions health` |
| Reject every pending trigger from this noisy rule (preview first) | `saas-alerts-cli rule-triggers decide --decision reject --rule <rule-id> --remarks "false positive"` |
| Which inbox rules exist at this customer? | `saas-alerts-cli reports mailbox-rules --organization-ids <customer-id>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/saas-alerts/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/saas-alerts/guide.md).

## What makes this one different

[WYRE-AI's saas-alerts-mcp](https://github.com/WYRE-AI/saas-alerts-mcp), which also backs the SaaS Alerts plugin in their msp-claude-plugins marketplace, exposes the SaaS Alerts External Partner API as 30 tools across customers, events, users, devices, billing, scheduled reports and partner settings, with no Respond, suppression or webhook tools. This connector covers all four SaaS Alerts APIs with one partner key, including Respond (it mints the Respond access token itself) and Webhooks, and syncs events into a local store so triage, cross-customer correlation and coverage gaps are one local query across every customer.

It complements the SaaS Alerts dashboard rather than replacing it: the dashboard stays the place to configure monitoring, review alerts and set up Respond, while this skill brings cross-customer triage, the Respond queue by deadline, one-account investigations and coverage gaps to whichever AI agent you already use.

## The pain this closes

- Respond's manual-approval queue is on a clock. The Respond API documents that a manual approval "will auto expire when the rule triggers again or 12 hours whichever is earlier", and the SaaS Alerts help center ("Analyzing a Respond trigger") says a failed remediation is reminded every 15 minutes and automatically ignored after the third reminder. Working that queue across many customers means knowing which decision lapses first.
- The Respond API needs a second credential, and its error hides that. In the r/msp thread "Kaseya Saas Alerts API" an MSP on the EU instance got "Only secure origins are allowed" on every call, and another replied they had been "banging my head against this same issue for weeks", suspecting CORS, with support tickets unanswered. The fix the thread landed on: POST to the auth endpoint first, then send the access token on every Respond call.
- The noise is real. MSP reviewers of SaaS Alerts on G2 describe a flood of notifications until the configuration is tuned and false positives you learn to filter out over time, and Kaseya's own Tech Jam webinar spends a session on noise reduction. Tuning well means seeing which alerts repeat across customers, not one tenant at a time.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/saas-alerts/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/saas-alerts/install.ps1 | iex
```

After install, authenticate once with your SaaS Alerts credentials, then verify with `saas-alerts-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | saas-alerts-cli triage; investigate; cross-tenant; coverage; rule-triggers queue; subscriptions health; rule-triggers decide and contain without --confirm; customers list; events list; events query; users; reports customers; reports mfa-microsoft; reports mailbox-rules; rules list; rule-triggers list; suppressions list; subscriptions list | Allow |
| Account response | contain --confirm; rule-triggers decide --confirm; rule-triggers approve; rule-triggers reject; rule-triggers ignore; rule-triggers manually-remediate; response-actions execute | Human-in-the-loop, explicit confirmation. These lock real people out. |
| Detection and Respond config | customers set-whitelists; customers set-account-whitelists; suppressions create; suppressions update; suppressions delete; rules update; rules draft; rules duplicate; rule-templates use; iocs update; respond enable; application-connections connect; subscriptions create; subscriptions update; subscriptions health --reenable --confirm; webhook-domains verify | Preview with --dry-run, then a human-approved write. The whitelist commands replace the customer's list, so read it first. |
| Write (routine) | devices map; devices unmap; psa map; psa map-suggested; customers create; customers update; reports scheduled-create; reports update-branding; subscriptions test-event; files upload | Preview with --dry-run, then a reviewed write |
| Credential and access | files url (returns a signed link to the stored file); auth set-token; auth logout | Human-in-the-loop only |
| Destructive and outbound | respond disable; customers delete; rules delete; application-connections delete; subscriptions delete; webhook-domains delete; webhook-queue delete; reports scheduled-delete; files delete; reports send-email; import | Human-in-the-loop only, explicit confirmation |

The skill authenticates with your SaaS Alerts partner API key, and that one key reaches every customer and all four APIs; SaaS Alerts documents no read-only or per-customer key. Triage, investigate, cross-tenant, coverage, the Respond queue and every list, get or report are read-only and safe to let an agent run. Approving Respond triggers, contain and response actions lock real Microsoft 365 users out, and whitelists, suppressions and Respond rules change what gets detected, so those need a human to approve the exact command. Generated commands run immediately, so set SAAS_ALERTS_MCP_READ_ONLY=1 for an agent that should only read. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/saas-alerts/governance.md).

## Frequently asked questions

### Is there an MCP server for SaaS Alerts?

Yes - this one. A free, open source MCP server and Claude Code Skill for SaaS Alerts, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the SaaS Alerts MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SAAS_ALERTS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SAAS_ALERTS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Your partner API key stays in your environment unless you save it with auth set-token, which writes it to a permission-restricted credentials file on your machine, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way saas-alerts-mcp runs, and shares MCP configuration with Codex: add saas-alerts-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my SaaS Alerts data safe?

Your data stays on your machine. The CLI, the MCP server and the local store are all local, and the only network calls go to SaaS Alerts and to destinations you name. The local store holds your customers' account emails, sign-in IPs and event details, so treat your data directory like any other copy of that data. Each webhook subscription's delivery token is withheld before it is printed, cached or stored, and credentials are never bundled or transmitted by MSP Skills.

### Where do I get the SaaS Alerts API key?

In the SaaS Alerts dashboard, open Settings > API and click GET APIKEY. SaaS Alerts shows the key only until you leave that page, so copy it then. Set it as SAAS_ALERTS_API_KEY. That one partner key reaches the Reports, Manage and Webhooks APIs, and the CLI exchanges it for the Respond access token on first use, so there is no second credential to paste. If every call answers 401 Access denied, ask SaaS Alerts support to enable the REST API for your account.

### Why does the Respond API answer "Only secure origins are allowed"?

That is what Respond returns when a call carries the api_key but no access token. Respond needs a short-lived token minted from the partner key with POST /respondApi/api/v1/auth. This connector mints it on the first Respond call, sends it on every Respond request, keeps it in memory for that process only, and mints a fresh one once if Respond rejects it.

### Can the AI block a user or approve Respond actions by itself?

Only if you let it. contain, rule-triggers decide and subscriptions health --reenable preview by default and change nothing until you add --confirm. The generated commands (rule-triggers approve, response-actions execute, customers delete and the rest) send as soon as they run, so preview them with --dry-run, which sends nothing. Over MCP, set SAAS_ALERTS_MCP_READ_ONLY=1 for an agent that should only read: saas-alerts_execute then runs GET endpoints only and the three workflow writers refuse --confirm.

### Do Respond triggers expire?

Yes. The Respond API documents that a manual approval expires after 12 hours or when the rule triggers again, whichever is earlier, and the SaaS Alerts help center says a failed remediation is automatically ignored after the third 15-minute reminder. rule-triggers queue lists pending and failed triggers by that deadline, with the actions an approval would fire, and rule-triggers decide skips expired or superseded triggers unless you pass --include-expired. The API does not say whether "triggers again" means anywhere or for the same customer, so the CLI counts a trigger as superseded only by a newer one of the same rule for the same organization and accounts: it may show a trigger Respond already expired (Respond then refuses the approve), but it never hides a live one.

### Why does triage say synced: false?

triage, investigate, cross-tenant and coverage read the local store, so they need a sync first, for example `saas-alerts-cli sync --resources customers,events --since 7d`. They say synced: false, with a note naming the fix, when the data they need was never synced, when the last events sync did not finish (an automatic refresh fetches one page at a time), or when the window you asked about starts before the oldest events you synced. A sync narrowed with --param stores its rows but does not count as complete. SaaS Alerts can record an event some time after it happened, so each explicit `sync --resources events` re-reads the 6 hours before the previous explicit sync; an event recorded later than that shows up after a sync with a wider --since. An empty or partial result is never presented as an all-clear, and --select always keeps the synced and note fields.

### Does contain work for Google Workspace accounts?

No. Respond response actions run against Microsoft 365 only, so contain refuses a Google Workspace or other non-Microsoft account, and stops when an email matches more than one Microsoft 365 account. The API returns no job id for an action, so contain reports each job match as best-effort and always says verified: false; check the account in the Microsoft 365 admin center.

### Has it been tested against a live SaaS Alerts tenant?

Not yet. Every endpoint traces to SaaS Alerts' published API specs and the Respond token exchange was checked against the live API without a key, but no partner has run it against real data yet. Two things wait on a live tenant: a partially rejected sync is reported as a sync_warning, so sync --strict can still exit 0 when some pages were refused; and the Unify device lists page with searchAfter from the response's meta.sort as the Manage spec describes, but if a tenant returns a bare list instead, a partner with more than 1,000 devices cannot sync them completely (sync and devices list --all then say truncated, never complete). Run it, and tell us what you find.

### Does it replace the SaaS Alerts dashboard?

No. The dashboard stays best for configuring monitoring, reviewing individual alerts and setting up Respond. This skill answers the across-every-customer questions and runs reviewed actions from your AI agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the SaaS Alerts API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
