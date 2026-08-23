---
layout: default
title: "Avanan MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Avanan (Check Point Harmony Email and Collaboration) API operation, plus shift-start triage, phishing campaign clustering, single-message lifecycle timelines, one exception lookup across all seven security engines, and cross-tenant MSP fleet rollups that a stateless API mirror cannot answer in a single call."
permalink: /skills/avanan/
skill_name: "Avanan MCP"
image: /assets/social/avanan/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for Avanan?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Avanan, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Avanan MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Avanan MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Avanan application ID and client secret once."
  - q: "Is my Avanan data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the MCP server speaks stdio only so it opens no network listener. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Message bodies and raw .eml files are only fetched when you explicitly ask for a named message."
  - q: "Will this hit my Avanan API rate limits?"
    a: "The skill syncs once into a local SQLite mirror, then answers most questions from local data, so the offline commands answer without spending API quota. Avanan publishes no quota numbers but does return HTTP 429, so the client backs off on its own and the offline commands (triage, campaign, timeline, exceptions find, exceptions audit, msp fleet) cost nothing after the sync."
  - q: "Do I need an MSP account, or does this work on a single tenant?"
    a: "Both. The MSP commands and the fleet rollup need an application credential bound to multiple tenants, but triage, campaign, timeline, exception lookup, search, and remediation all work fine against a single-tenant credential."
  - q: "Why does my Avanan credential fail against a different region?"
    a: "Avanan regions are hard-isolated and credentials are issued per region, so a USA key cannot read EU data by design. Set the region once with avanan-cli auth login --region <us|eu|ca|ap|uk|uae|in> --save. avanan-cli doctor prints the host it resolved."
  - q: "Why do quarantine and restore ask for a scope?"
    a: "Avanan's action endpoints accept exactly one scope and reject a multi-tenant credential with a bare HTTP 400 that reads like a bad key. The remediate command turns that into an error naming the scopes your credential actually covers, so a mailbox-affecting write cannot land on a tenant you did not name."
  - q: "Does it cover Check Point firewall or endpoint products?"
    a: "No. This covers Avanan / Harmony Email and Collaboration only: email and SaaS collaboration security. Check Point system and audit logs live on the separate Infinity Portal API and are not part of this one."
  - q: "Will this replace the Avanan portal?"
    a: "No. Security policy rules and engine configuration are portal-only; the API exposes exceptions, not policy. This is for the questions and the batch work the portal makes slow."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/avanan/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Avanan credentials once; avanan-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Avanan question in plain language; it runs avanan-cli for you."
---

# The Avanan MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Check Point Software Technologies Ltd..

**✓ Live-verified by @geekbrownbear (Bearium Networks, MSP)** against a production tenant · 2026-08-23 · [receipt →](https://github.com/Servosity/msp-skills/pull/271).

Yes - there is an MCP server for Avanan. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Avanan to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI what Avanan caught this morning and get a bucketed digest across every tenant you manage, not a page of raw detections you have to re-query. It reads the whole book of business at once: which sender domains are driving the volume, whether forty detections are one phishing campaign or forty separate problems, what the local mirror recorded happening to the message a user is disputing, and whether the domain somebody wants allowlisted is already excepted in one of the seven engines nobody remembers to check.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/avanan){: .btn}

## Instead of clicking through Avanan, just ask

**Instead of** Re-run the same event query in the portal every hour and eyeball which detections you already handled
**just ask:** *"What did Avanan catch on my tenants in the last 24 hours?"*
<sub>Your agent runs: <code>avanan-cli triage --since 24h</code></sub>

**Instead of** Sort a detection export by sender and subject in a spreadsheet to work out whether it is one campaign
**just ask:** *"Is this one phishing campaign or forty separate problems?"*
<sub>Your agent runs: <code>avanan-cli campaign --since 7d</code></sub>

**Instead of** Open anti-phishing, spam, anomaly, click-time protection, anti-malware, URL reputation, and DLP one at a time to see if a domain is already allowlisted
**just ask:** *"Is this domain excepted anywhere in Avanan?"*
<sub>Your agent runs: <code>avanan-cli exceptions find example.com</code></sub>


## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What did Avanan catch across my tenants since the start of my shift? | `avanan-cli triage --since 24h` |
| Are these forty detections one phishing campaign or forty problems? | `avanan-cli campaign --since 7d` |
| A user says we quarantined a legitimate email. What actually happened to it? | `avanan-cli timeline <entity-id>` |
| Is this domain, sender, URL, or hash already excepted anywhere? | `avanan-cli exceptions find example.com` |
| Which of our exceptions contradict each other or have not matched any traffic in the mirrored window? | `avanan-cli exceptions audit` |
| Which tenant is over its seat count or having an unusual detection week? | `avanan-cli msp fleet` |
| Quarantine this batch and tell me the real per-message outcome, not just that a task started. | `avanan-cli remediate quarantine --entity <entity-id> --scope <farm:tenant> --wait` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/avanan/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/avanan/guide.md).

## What makes this one different

Every other Avanan integration is stateless, so it can answer one question about one tenant right now. This skill syncs events, entities, exceptions, and MSP objects into a local SQLite mirror and answers the questions that need history and fan-out as one offline join: a bucketed triage digest, campaign clustering by sender domain and normalized subject, a single message's history in order, as far as the local mirror can attest, and one exception lookup across all seven engines and nine exception tables at once. It also implements the documented request signature exactly, including the request-string term the published docs omit, where the leading community MCP server ships a self-declared best-guess HMAC it warns users to replace.

The Avanan portal shows one tenant's detections, one engine's exception list, one message's current state. This skill answers the questions that span all of them at once - what came in across the fleet, which detections are the same campaign, where a domain is already allowlisted, which tenant is over its seats - and it turns quarantine and restore from fire-and-poll into a command that waits for the task and reports the real per-item outcome.

## The pain this closes

- Every existing Avanan integration is a plugin inside XSOAR, Sentinel, n8n, or an MCP host. All of them are stateless: one question per API call, no history, and no way to ask anything that spans two tenants or two days without paying for N more calls against a rate-limited API.
- Allow and block exceptions live in seven separate security engines with different path shapes, ID schemes, and delete semantics. Nothing in the product answers 'is this domain already excepted', so allowlist requests get granted twice, contradicted across engines, and never cleaned up.
- Quarantine and restore are asynchronous: the action endpoint returns a task ID and the real per-item outcome only appears when that task finishes. Every existing tool hands the polling back to you, and the action endpoints reject a multi-tenant credential with a bare HTTP 400 that reads like a broken key.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/avanan/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/avanan/install.ps1 | iex
```

After install, authenticate once with your Avanan credentials, then verify with `avanan-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, campaign, timeline, exceptions find, exceptions audit, msp fleet, event query, event get, avanan-search query-saas-entity, scopes, task, msp list and describe commands, sectool and sectools list/get commands, sync, mirror, search, analytics, export | Allow |
| Message content egress | download, download-large-email, avanan-search get-saas-entity (soar get-entity currently returns 404 on every tenant tested) | Allow only for a named message under an open investigation; never sweep |
| Write (routine) | exceptions create, exceptions create-whitelist, exceptions update, exceptions update-whitelist, sectool-exceptions create, sectool-exceptions update, sectool-exceptions exceptions create-sectool-entry, sectools create-anomaly-exception, sectools create-ctp-item, sectools update-ctp-item, sectools update-ctp-items, report | Preview with --dry-run, then a reviewed write |
| Bulk write (import) | import (bulk POST from JSONL; import action reaches live mail, import soar notifies end users, import msp mutates tenants) | Human-approved only; inherits the tier of the resource it targets. Preview with --dry-run first |
| Mailbox-affecting | remediate quarantine, remediate restore, action post-entity, action post-event | Preview with --dry-run, then a human-approved write, always with an explicit --scope |
| End-user contact | soar post-notify | Human-in-the-loop only |
| Tenant and billing lifecycle | msp create, msp create-tenants, msp create-users, msp update, msp update-or-create-tenant-license | Human-in-the-loop only |
| Destructive | msp delete, msp delete-tenants, msp delete-users, exceptions delete, sectool-exceptions delete, sectool-exceptions exceptions delete-sectool-entries, sectools delete-anomaly-exceptions, sectools delete-ctp-item, sectools delete-ctp-items, sectools delete-ctp-lists | Human-in-the-loop only, explicit confirmation |
| Credential / security | auth login, auth set-token, auth logout | Operator-only, not for agents |

The skill reads your Avanan detections, scanned messages and files, exceptions across all seven security engines, tenants, licenses, and usage, and it can create and delete exceptions, quarantine and restore live mail, report a mis-classification, notify an end user, download the raw .eml for a message, and create, update, or delete MSP tenants, users, and license assignments. Reads and sync cannot change anything, but safe to run is not the same as safe to sweep: the message-content reads listed in the tier table pull real customer email onto your machine, and `import` replays a file as live POSTs. Exception writes and reclassification should be previewed with --dry-run, then approved. Quarantine and restore reach into a real mailbox and should be human-approved with an explicit scope. Downloading message bodies is a privacy decision, not a lookup. Notifying end users, changing tenants or licenses, and every delete are human-in-the-loop only. The strongest control is the credential itself: Avanan application credentials are region-scoped and tenant-scoped, so a credential bound to one tenant puts the whole fleet out of reach. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/avanan/governance.md).

## Frequently asked questions

### Is there an MCP server for Avanan?

Yes - this one. A free, open source MCP server and Claude Code Skill for Avanan, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Avanan MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Avanan MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Avanan application ID and client secret once.

### Is my Avanan data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the MCP server speaks stdio only so it opens no network listener. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Message bodies and raw .eml files are only fetched when you explicitly ask for a named message.

### Will this hit my Avanan API rate limits?

The skill syncs once into a local SQLite mirror, then answers most questions from local data, so the offline commands answer without spending API quota. Avanan publishes no quota numbers but does return HTTP 429, so the client backs off on its own and the offline commands (triage, campaign, timeline, exceptions find, exceptions audit, msp fleet) cost nothing after the sync.

### Do I need an MSP account, or does this work on a single tenant?

Both. The MSP commands and the fleet rollup need an application credential bound to multiple tenants, but triage, campaign, timeline, exception lookup, search, and remediation all work fine against a single-tenant credential.

### Why does my Avanan credential fail against a different region?

Avanan regions are hard-isolated and credentials are issued per region, so a USA key cannot read EU data by design. Set the region once with avanan-cli auth login --region <us|eu|ca|ap|uk|uae|in> --save. avanan-cli doctor prints the host it resolved.

### Why do quarantine and restore ask for a scope?

Avanan's action endpoints accept exactly one scope and reject a multi-tenant credential with a bare HTTP 400 that reads like a bad key. The remediate command turns that into an error naming the scopes your credential actually covers, so a mailbox-affecting write cannot land on a tenant you did not name.

### Does it cover Check Point firewall or endpoint products?

No. This covers Avanan / Harmony Email and Collaboration only: email and SaaS collaboration security. Check Point system and audit logs live on the separate Infinity Portal API and are not part of this one.

### Will this replace the Avanan portal?

No. Security policy rules and engine configuration are portal-only; the API exposes exceptions, not policy. This is for the questions and the batch work the portal makes slow.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the Avanan API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
