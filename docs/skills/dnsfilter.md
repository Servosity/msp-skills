---
layout: default
title: "DNSFilter MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every DNSFilter API operation, plus one-command answers to 'why is this site blocked for this client' and 'which clients are behind on the roaming agent'."
permalink: /skills/dnsfilter/
skill_name: "DNSFilter MCP"
image: /assets/social/dnsfilter/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for DNSFilter?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for DNSFilter, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the DNSFilter MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DNSFILTER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DNSFILTER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and dnsfilter-mcp speaks HTTP natively: run `dnsfilter-mcp --transport http` with DNSFILTER_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header, since ChatGPT connectors do not send a fixed one. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my DNSFilter data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Secret fields DNSFilter returns, such as each site's secret key and API key tokens, are not written to the local mirror."
  - q: "What DNSFilter API key do I need?"
    a: "An API token created in the DNSFilter dashboard (see DNSFilter's API Tokens help article). Set DNSFILTER_API_KEY to the token itself: DNSFilter expects the bare token in the Authorization header, with no Bearer prefix. Create it in your MSP organization so the cross-client commands can see your client organizations; a token from a single client organization only sees that client."
  - q: "Can the AI allow a domain or change a policy by itself?"
    a: "Only if you let it. allow prints the plan and writes nothing unless you add --confirm with a --note (the ticket number), refuses a domain in a security category unless you add --force, skips policies you cannot edit and leaves shared global policies alone unless you pass --include-global; a saved profile can never supply --confirm or --force. Generated commands such as policies update or policies add-whitelist-domain send as soon as they run, so preview them with --dry-run. Over MCP, the generic dnsfilter_execute tool can call any DNSFilter endpoint, deletes included, gated only by your AI app's approval prompt. For an agent that should only read, set DNSFILTER_MCP_READ_ONLY=1 in the MCP server's environment: write tools and allow are removed, dnsfilter_execute runs GET endpoints only, and API key reads, site secret keys and the roaming client uninstall PIN are withheld. Keep the approval prompt on."
  - q: "Does why-blocked see every rule DNSFilter applies?"
    a: "No, and it says so. It evaluates the site or roaming client's current policy (including the scheduled-policy slot and a roaming client inheriting its site's policy) in DNSFilter's documented order: allow list, allow-list-only mode, block list, uncategorized domains, blocked categories. User, collection and LAN-subnet assignments, the org or MSP Universal Allow and Block Lists, and AppAware application rules are not exposed by the policy API, so they are not evaluated; when a Query Log hit names a policy outside the ones it checked, the output flags it."
  - q: "Why do agents drift, agents stale or lists where say there is nothing to report?"
    a: "They read the local mirror, so run sync first: `dnsfilter-cli sync --resources organizations,networks,policies,scheduled-policies,user-agents`. agents drift also reads DNSFilter's release feed live on every run, so a pulled release does not linger in a stale copy."
  - q: "Has it been tested against a live DNSFilter account?"
    a: "Not yet. It is built from DNSFilter's published OpenAPI spec and tested against a local mock, so it ships as Beta and Awaiting live verification. A few request shapes were inferred from the spec (some list filters sent in the request body, the bulk domain lookup requiring fqdns) and will be confirmed on the first live run; tell us what you see."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your DNSFilter credentials once, then run dnsfilter-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a DNSFilter question in plain language; it runs dnsfilter-cli for you."
---

# The DNSFilter MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by DNSFilter, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for DNSFilter. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects DNSFilter to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running DNSFilter for a book of clients? Ask your AI "why is this site blocked for Acme Dental," "which unblock requests are waiting across my clients," or "who is still on the roaming client build that got pulled" and get one answer instead of clicking through each organization. why-blocked names the policy and rule that decided it, allow previews the fix on the policies that actually blocked it, and the fleet views cover every client org.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/dnsfilter){: .btn}

## Instead of clicking through DNSFilter, just ask

**Instead of** Opening the client's organization in the DNSFilter dashboard, finding which policy its site or roaming client uses right now, then reading the Query Log to work out which rule blocked the site the user is calling about
**just ask:** *"Why is example.com blocked for Acme Dental, and which policy did it?"*
<sub>Your agent runs: <code>dnsfilter-cli why-blocked "example.com" --org "Acme Dental"</code></sub>

**Instead of** Working through end-user unblock requests one client at a time, looking up each domain's category and checking whether it is already allowed before deciding
**just ask:** *"What unblock requests are waiting across my clients, and which are threats or already allowed?"*
<sub>Your agent runs: <code>dnsfilter-cli unblocks triage</code></sub>

**Instead of** After a roaming client release is pulled, filtering the Roaming Clients page organization by organization to find the machines still on that version
**just ask:** *"Which clients still have Windows roaming clients on version 1.7.15?"*
<sub>Your agent runs: <code>dnsfilter-cli agents drift --bad-version 1.7.15 --agent-type windows</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/dnsfilter/wide-1200x630.png" src="/assets/video/dnsfilter/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/dnsfilter/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why is this domain blocked for this client, and which policy and rule did it? | `dnsfilter-cli why-blocked "example.com" --org "Acme Dental"` |
| Allow it on the policies that actually blocked it (preview first) | `dnsfilter-cli allow "example.com" --org "Acme Dental" --note "TKT-4821"` |
| What unblock requests are waiting across every client, grouped by domain? | `dnsfilter-cli unblocks triage` |
| Which clients have roaming clients behind the latest agent release? | `dnsfilter-cli agents drift` |
| Who is still on the roaming client build that got pulled? | `dnsfilter-cli agents drift --bad-version 1.7.15 --agent-type windows` |
| Which roaming clients stopped checking in, or are unprotected or bypassed? | `dnsfilter-cli agents stale --older-than 7d` |
| Which client policies already allow or block this domain or a parent domain? | `dnsfilter-cli lists where "example.com"` |
| Which allow-listed domains are now categorized as threats, and which policies leave security categories unblocked? | `dnsfilter-cli policy-audit` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/dnsfilter/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/dnsfilter/guide.md).

## What makes this one different

A plain DNSFilter API wrapper answers one endpoint per call, so "why is this blocked" turns into the AI chaining an organization lookup, site and roaming-client policy assignments, the current scheduled-policy slot, the policy's lists and categories, a domain lookup and the Query Log on its own. why-blocked and allow do that chain in one command and say what they could not evaluate. agents drift, agents stale and lists where answer from a local SQLite mirror that sync fills, so a fleet-wide question is one local query across every client org.

It complements the DNSFilter dashboard rather than replacing it: policies, deployments, reporting and the Query Log stay there, while this skill brings cross-client troubleshooting, the unblock queue, roaming client drift and preview-first allow-list changes to whichever AI agent you already use.

## The pain this closes

- A blocked-site call is a chain, not a lookup. The answer depends on which policy the client's site or roaming client is using at that moment (scheduled policies change it by the quarter hour), that policy's allow list, block list and blocked categories, and what the Query Log recorded. A DNSFilter Community post, "Policy enforcement & reporting discrepancies with Roaming Client", describes a roaming client on a second site ignoring its category blocks, and reaching a URL that its reports showed as blocked.
- Roaming client releases go wrong across the whole book at once. DNSFilter's own status page logged the Windows Roaming Client 1.7.15 auto-upgrade incident: auto-upgrade was paused, the release was pulled from general availability, and the workaround was to uninstall and reinstall the client. DNSFilter's help center also documents macOS machines losing internet connectivity when updating the Roaming Client from 1.x to 2.2. The first question each time is which clients, and which machines, are on the bad build.
- Unblock requests and allow-list changes pile up per client. Each request means checking the domain's category, whether some policy or Universal List already allows it, and which policy to edit, and an allow added to the wrong policy (or a shared global one) opens the site for clients who never asked.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/dnsfilter/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/dnsfilter/install.ps1 | iex
```

After install, authenticate once with your DNSFilter credentials, then verify with `dnsfilter-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | dnsfilter-cli why-blocked; unblocks triage; agents drift; agents stale; lists where; policy-audit; allow (without --confirm, plan only); organizations index; organizations show; policies index; policies show; user-agents index; user-agents tags; unblock-requests index; traffic-reports query-logs; domains user-lookup; search; sync | Allow |
| Filtering policy and allow lists | allow --confirm; policies add-whitelist-domain; policies remove-whitelist-domain; policies add-blacklist-domain; policies remove-blacklist-domain; policies add-blacklist-category; policies remove-blacklist-category; policies bulk-actions-add-allowlist-domains; policies update; policies permissive-mode policies-update; policies add-allowed-application; scheduled-policies update; unblock-requests decision unblock-requests; block-pages update | Preview (allow without --confirm, or --dry-run), then a human-approved write. One allow or a weaker policy opens the site for every deployment on that policy. |
| Write (routine) | networks create; networks update; organizations create; organizations update; user-agents update; ip-addresses create; mac-addresses create; scheduled-reports create; dashboards create; notes domain-update | Preview with --dry-run, then a reviewed write |
| Credential and access | networks index, networks show, networks all, networks msp and networks msp-all (rows carry each site's secret key); networks secret-key networks-generate; networks secret-key networks-rotate; networks secret-key networks-revoke; api-keys index; api-keys show; api-keys create; api-keys revoke api-keys; user-agents uninstall-pin; users change-password; distributors users-reset-password-url; organizations users organization-create; psa-integrations (a GET that creates a PSA integration and returns its link); auth set-token | Human-in-the-loop only |
| Destructive and commercial | import (one create per JSONL line, so it can reach any tier above); policies destroy; networks destroy; networks bulk-destroy; organizations destroy; organizations cancel; user-agents destroy; agent-local-user-bulk-deletes create; api-keys destroy; billing create; billing address-update; distributors organizations-cancel; distributors msps-update; trials | Human-in-the-loop only, explicit confirmation |

The skill authenticates with a DNSFilter API token you create, and it can do whatever that token can do in the dashboard. Every troubleshooting and fleet view (why-blocked, unblocks triage, agents drift, agents stale, lists where, policy-audit) and the ordinary lists and gets are read-only and safe to let an agent run. allow previews by default; it and every policy, allow-list, block-list, scheduled-policy and unblock-decision change need a human to approve the exact command, because one allow opens the site for every site and roaming client on that policy. Reads that return a secret (site secret keys in the networks lists, API key tokens, the roaming client uninstall PIN) and everything that creates, rotates or revokes a credential are human-only, as are deletes, roaming client uninstalls, billing and distributor changes. The MCP server's generic execute tool can call any endpoint, so set DNSFILTER_MCP_READ_ONLY=1 for an agent that should only read. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/dnsfilter/governance.md).

## Frequently asked questions

### Is there an MCP server for DNSFilter?

Yes - this one. A free, open source MCP server and Claude Code Skill for DNSFilter, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the DNSFilter MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DNSFILTER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DNSFILTER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and dnsfilter-mcp speaks HTTP natively: run `dnsfilter-mcp --transport http` with DNSFILTER_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header, since ChatGPT connectors do not send a fixed one. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my DNSFilter data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Secret fields DNSFilter returns, such as each site's secret key and API key tokens, are not written to the local mirror.

### What DNSFilter API key do I need?

An API token created in the DNSFilter dashboard (see DNSFilter's API Tokens help article). Set DNSFILTER_API_KEY to the token itself: DNSFilter expects the bare token in the Authorization header, with no Bearer prefix. Create it in your MSP organization so the cross-client commands can see your client organizations; a token from a single client organization only sees that client.

### Can the AI allow a domain or change a policy by itself?

Only if you let it. allow prints the plan and writes nothing unless you add --confirm with a --note (the ticket number), refuses a domain in a security category unless you add --force, skips policies you cannot edit and leaves shared global policies alone unless you pass --include-global; a saved profile can never supply --confirm or --force. Generated commands such as policies update or policies add-whitelist-domain send as soon as they run, so preview them with --dry-run. Over MCP, the generic dnsfilter_execute tool can call any DNSFilter endpoint, deletes included, gated only by your AI app's approval prompt. For an agent that should only read, set DNSFILTER_MCP_READ_ONLY=1 in the MCP server's environment: write tools and allow are removed, dnsfilter_execute runs GET endpoints only, and API key reads, site secret keys and the roaming client uninstall PIN are withheld. Keep the approval prompt on.

### Does why-blocked see every rule DNSFilter applies?

No, and it says so. It evaluates the site or roaming client's current policy (including the scheduled-policy slot and a roaming client inheriting its site's policy) in DNSFilter's documented order: allow list, allow-list-only mode, block list, uncategorized domains, blocked categories. User, collection and LAN-subnet assignments, the org or MSP Universal Allow and Block Lists, and AppAware application rules are not exposed by the policy API, so they are not evaluated; when a Query Log hit names a policy outside the ones it checked, the output flags it.

### Why do agents drift, agents stale or lists where say there is nothing to report?

They read the local mirror, so run sync first: `dnsfilter-cli sync --resources organizations,networks,policies,scheduled-policies,user-agents`. agents drift also reads DNSFilter's release feed live on every run, so a pulled release does not linger in a stale copy.

### Has it been tested against a live DNSFilter account?

Not yet. It is built from DNSFilter's published OpenAPI spec and tested against a local mock, so it ships as Beta and Awaiting live verification. A few request shapes were inferred from the spec (some list filters sent in the request body, the bulk domain lookup requiring fqdns) and will be confirmed on the first live run; tell us what you see.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the DNSFilter API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
