---
layout: default
title: "Auvik MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Auvik endpoint as a command, plus the cross-client answers the Auvik UI and API cannot give you."
permalink: /skills/auvik/
skill_name: "Auvik MCP"
image: /assets/social/auvik/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Auvik?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Auvik, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Auvik MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Auvik MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Auvik data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "TODO: vendor-specific question MSP owners actually search (rate limits, partner requirements, replacing the Auvik portal)"
    a: "TODO"
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Auvik credentials once; auvik-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Auvik question in plain language; it runs auvik-cli for you."
---

# The Auvik MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Auvik Networks Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Auvik. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Auvik to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

TODO: <=70 words, MSP-owner language, leads with the outcome. What does Auvik + your AI answer in one sentence that the portal cannot?

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/auvik){: .btn}

## Instead of clicking through Auvik, just ask

**Instead of** TODO: the painful manual workflow (exporting reports, clicking through the portal)
**just ask:** *"TODO: the natural-language question the MSP owner asks instead"*
<sub>Your agent runs: <code>auvik-cli TODO</code></sub>

**Instead of** TODO
**just ask:** *"TODO"*
<sub>Your agent runs: <code>auvik-cli TODO</code></sub>

**Instead of** TODO
**just ask:** *"TODO"*
<sub>Your agent runs: <code>auvik-cli TODO</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/auvik/wide-1200x630.png" src="/assets/video/auvik/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/auvik/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| TODO: question an MSP keeps asking | `auvik-cli TODO` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/auvik/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/auvik/guide.md).

## What makes this one different

TODO: one or two sentences vs typical MCP wrappers (generic, no competitor names): most Auvik integrations proxy each question into a live API call ...

TODO: one sentence vs Auvik's own AI features (complements, not replaces). If the vendor has no AI integration, say what this adds that the portal cannot.

## The pain this closes

- TODO: pain 1 in MSP-owner vocabulary, sourced from a real community thread
- TODO: pain 2

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/auvik/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/auvik/install.ps1 | iex
```

After install, authenticate once with your Auvik credentials, then verify with `auvik-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | TODO: read commands | Allow |
| Write (routine) | TODO | Preview with --dry-run, then a reviewed write |
| Destructive / config | TODO | Human-in-the-loop only |

TODO: 2-3 plain-language sentences from governance.md - what the skill can read, what it can change, and the recommended agent policy per tier. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/auvik/governance.md).

## Frequently asked questions

### Is there an MCP server for Auvik?

Yes - this one. A free, open source MCP server and Claude Code Skill for Auvik, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Auvik MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Auvik MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Auvik data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### TODO: vendor-specific question MSP owners actually search (rate limits, partner requirements, replacing the Auvik portal)

TODO


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Auvik API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
