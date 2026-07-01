---
layout: default
title: "AWS MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Get a non-expert from zero to a Slack-delivered, plain-English, waste-flagged AWS bill breakdown  -  and cache it so you don't pay per Cost Explorer call."
permalink: /skills/aws-billing/
skill_name: "AWS MCP"
image: /assets/social/aws-billing/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for AWS?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for AWS, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the AWS MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local AWS MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your AWS credentials once."
  - q: "Is my AWS data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this run up my Cost Explorer bill?"
    a: "No - that is the point of the local cache. 'sync' pulls your cost data once (each Cost Explorer request is about $0.01), then bill, compare, consolidated, waste, and ask answer from the local SQLite mirror for free. Pass --data-source live only when you want a fresh pull."
  - q: "Do I need the aws CLI installed?"
    a: "No. The binary signs its own AWS requests (SigV4) using the native credential chain - environment variables, a shared --profile-aws, SSO, assume-role, or instance metadata. There is nothing to paste and no aws CLI dependency."
  - q: "Does it work from a member account, or only the payer?"
    a: "Org-wide cost data (the consolidated rollup) needs a management/payer-account profile; from a member account you see only that account's own costs. Resource-level waste scans work in any account. Run 'aws-billing-cli doctor' to see exactly what your credentials can reach."
  - q: "Can it change anything in my AWS account?"
    a: "No. Every command is read-only against the AWS billing & Cost Explorer API - it never stops, deletes, modifies, or buys anything. 'waste gp2-gp3' even prints the 'aws ec2 modify-volume' command you would run rather than running it. The opt-in outbound network actions are 'report --post-slack' (Slack), 'feedback --send' (upstream note, only if you set an endpoint), and '--deliver webhook:<url>' (POSTs output to a URL you name) - none of them changes anything in AWS, and even the generic 'import' command can't, because the billing API exposes no write endpoint."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/aws-billing/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your AWS credentials once; aws-billing-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a AWS question in plain language; it runs aws-billing-cli for you."
---

# The AWS MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Amazon.com, Inc. or its affiliates.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for AWS. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects AWS to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "why did my AWS bill go up?" and get a plain-English answer: the service that moved, the linked account driving it, and the idle instances and orphaned volumes bleeding dollars - without learning Cost Explorer's dimension grammar. It syncs your bill into a local cache once, so every follow-up question is instant and free instead of $0.01 per Cost Explorer call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/aws-billing){: .btn}

## Instead of clicking through AWS, just ask

**Instead of** Export Cost Explorer CSVs and pivot them in a spreadsheet to find which service jumped this month
**just ask:** *"Why did my AWS bill go up since last month?"*
<sub>Your agent runs: <code>aws-billing-cli compare --from last-month --to this-month</code></sub>

**Instead of** Click through every linked account in the billing console to find the one driving org spend
**just ask:** *"Which account is driving my AWS spend?"*
<sub>Your agent runs: <code>aws-billing-cli consolidated --period this-month</code></sub>

**Instead of** Hunt through the EC2, EBS, and Elastic IP consoles by hand for idle and orphaned resources
**just ask:** *"Where am I wasting money on AWS?"*
<sub>Your agent runs: <code>aws-billing-cli waste rank</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/aws-billing/wide-1200x630.png" src="/assets/video/aws-billing/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/aws-billing/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why did my bill change month-over-month? | `aws-billing-cli compare --from last-month --to this-month` |
| Which linked account is driving org spend? | `aws-billing-cli consolidated --period this-month` |
| Where am I wasting money right now? | `aws-billing-cli waste rank` |
| What does this cryptic usage-type line mean? | `aws-billing-cli explain EUC1-DataTransfer-Out-Bytes` |
| Where is my data-transfer cost leaking? | `aws-billing-cli waste transfer --period last-month` |
| What will I spend next month? | `aws-billing-cli forecast --profile-aws prod` |
| How do I give a colleague read-only billing access? | `aws-billing-cli iam-setup --tier core --format cloudformation` |
| What's a plain answer about my bill? | `aws-billing-cli ask "what are my top services"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/aws-billing/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/aws-billing/guide.md).

## What makes this one different

Most AWS integrations proxy every question into a live Cost Explorer call - fine for one lookup, but each request costs $0.01 and it dies the moment you want a month-over-month trend or a cross-account rollup. This skill syncs your bill and resource inventory into a local SQLite mirror once, so compare, consolidated, and waste rank run instantly and offline, and the agent sees the ranked answer rather than a raw API dump.

AWS's console and Cost Explorer can show you the numbers, but they assume you already speak the dimension grammar and they steer you toward buying Reserved Instances. This skill decodes the bill into plain English, ranks real waste by dollars saved, and never tries to sell you a commitment - it complements the console for anyone who isn't a billing specialist.

## The pain this closes

- The AWS bill is a wall of usage-type codes like EUC1-DataTransfer-Out-Bytes that nobody on the team can read, so the monthly cost review keeps getting skipped until the bill is already a surprise.
- You suspect money is bleeding on idle instances and orphaned volumes, but Cost Explorer points you at buying Reserved Instances instead of naming the actual waste - and every report you pull costs $0.01.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/aws-billing/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/aws-billing/install.ps1 | iex
```

After install, authenticate once with your AWS credentials, then verify with `aws-billing-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | bill, consolidated, compare, forecast, waste rank, waste transfer, ask, explain, dimensions, doctor, iam-setup | Allow |
| Write (local) | sync (writes the local cache), report (writes an HTML/PDF file) | Allow; never mutates AWS |
| Outbound (opt-in) | report --post-slack (Slack post), feedback --send (upstream note, only with an endpoint set), --deliver webhook:<url> (POSTs output to a URL you name) | Allow; each fires only when you pass the flag |
| Destructive / config | none - the AWS billing & Cost Explorer API exposes no write endpoints, so even the generic import command cannot mutate AWS | N/A |

Every command is read-only against the AWS billing & Cost Explorer API: it pulls cost and inventory data and never stops an instance, deletes a volume, or buys a commitment. Local writes are the SQLite cache ('sync') and report files ('report'). Outbound actions are opt-in and explicit and none touches AWS: 'report --post-slack' (Slack), 'feedback --send' (upstream note, only if you set an endpoint), and '--deliver webhook:<url>' (POSTs output to a URL you name). Scope the AWS credentials to read-only - run 'iam-setup' to mint exactly that - and an agent can run the read surface unattended. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/aws-billing/governance.md).

## Frequently asked questions

### Is there an MCP server for AWS?

Yes - this one. A free, open source MCP server and Claude Code Skill for AWS, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the AWS MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local AWS MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your AWS credentials once.

### Is my AWS data safe?

Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this run up my Cost Explorer bill?

No - that is the point of the local cache. 'sync' pulls your cost data once (each Cost Explorer request is about $0.01), then bill, compare, consolidated, waste, and ask answer from the local SQLite mirror for free. Pass --data-source live only when you want a fresh pull.

### Do I need the aws CLI installed?

No. The binary signs its own AWS requests (SigV4) using the native credential chain - environment variables, a shared --profile-aws, SSO, assume-role, or instance metadata. There is nothing to paste and no aws CLI dependency.

### Does it work from a member account, or only the payer?

Org-wide cost data (the consolidated rollup) needs a management/payer-account profile; from a member account you see only that account's own costs. Resource-level waste scans work in any account. Run 'aws-billing-cli doctor' to see exactly what your credentials can reach.

### Can it change anything in my AWS account?

No. Every command is read-only against the AWS billing & Cost Explorer API - it never stops, deletes, modifies, or buys anything. 'waste gp2-gp3' even prints the 'aws ec2 modify-volume' command you would run rather than running it. The opt-in outbound network actions are 'report --post-slack' (Slack), 'feedback --send' (upstream note, only if you set an endpoint), and '--deliver webhook:<url>' (POSTs output to a URL you name) - none of them changes anything in AWS, and even the generic 'import' command can't, because the billing API exposes no write endpoint.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [Gradient MSP](/skills/gradient/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the AWS API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
