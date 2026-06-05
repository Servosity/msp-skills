---
layout: default
title: "ConnectWise PSA (Manage) MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every ConnectWise PSA workflow from the terminal  -  with a typed conditions query builder, offline SQLite sync, and cross-entity views (unbilled work, account 360, board triage) the PSA web UI can't give you."
permalink: /skills/connectwise-manage/
skill_name: "ConnectWise PSA (Manage) MCP"
image: /assets/social/connectwise-manage/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ConnectWise Manage MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ConnectWise Manage data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "TODO: vendor-specific question MSP owners actually search (rate limits, partner requirements, replacing the ConnectWise Manage portal)"
    a: "TODO"
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ConnectWise PSA (Manage) credentials once; connectwise-manage-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectWise PSA (Manage) question in plain language; it runs connectwise-manage-cli for you."
---

# ConnectWise PSA (Manage) + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectWise Manage
> API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

TODO: <=70 words, MSP-owner language, leads with the outcome. What does ConnectWise Manage + your AI answer in one sentence that the portal cannot?

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectwise-manage){: .btn}

## Instead of clicking through ConnectWise PSA (Manage), just ask

**Instead of** TODO: the painful manual workflow (exporting reports, clicking through the portal)
**just ask:** *"TODO: the natural-language question the MSP owner asks instead"*
<sub>Your agent runs: <code>connectwise-manage-cli TODO</code></sub>

**Instead of** TODO
**just ask:** *"TODO"*
<sub>Your agent runs: <code>connectwise-manage-cli TODO</code></sub>

**Instead of** TODO
**just ask:** *"TODO"*
<sub>Your agent runs: <code>connectwise-manage-cli TODO</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/connectwise-manage/wide-1200x630.png" src="/assets/video/connectwise-manage/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/connectwise-manage/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| TODO: question an MSP keeps asking | `connectwise-manage-cli TODO` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/guide.md).

## What makes this one different

TODO: one or two sentences vs typical MCP wrappers (generic, no competitor names): most ConnectWise Manage integrations proxy each question into a live API call ...

TODO: one sentence vs ConnectWise Manage's own AI features (complements, not replaces). If the vendor has no AI integration, say what this adds that the portal cannot.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-manage/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex
```

After install, authenticate once with your ConnectWise PSA (Manage) credentials, then verify with `connectwise-manage-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | TODO: read commands | Allow |
| Write (routine) | TODO | Preview with --dry-run, then a reviewed write |
| Destructive / config | TODO | Human-in-the-loop only |

TODO: 2-3 plain-language sentences from governance.md - what the skill can read, what it can change, and the recommended agent policy per tier. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ConnectWise Manage MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ConnectWise Manage data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### TODO: vendor-specific question MSP owners actually search (rate limits, partner requirements, replacing the ConnectWise Manage portal)

TODO


## Status

Beta. Validated against the ConnectWise PSA (Manage) API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
