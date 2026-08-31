---
layout: default
title: "How do MSPs use Claude and ChatGPT with their PSA? | MSP Skills"
description: "How MSPs connect Claude, ChatGPT, Codex, and Copilot to their PSA, RMM, and backup tools: install a free MCP server connector that runs locally, then ask plain-English questions across every client. The AI runs the underlying command for you - triage, client cards, SLA breaches, cross-client analytics - no code required."
permalink: /answers/how-msps-use-claude-and-chatgpt-with-their-psa/
faqs:
  - q: "How do MSPs use Claude and ChatGPT with their PSA?"
    a: "They install a free MCP server connector for the PSA that runs locally on their own machine, then ask plain-English questions in Claude, ChatGPT, Codex, or Copilot. The AI runs the underlying command for them - triage the queue, build a client card, find SLA breaches, run cross-client analytics. No code is required; you paste one sentence to install and then just ask questions."
  - q: "Can ChatGPT connect to my PSA?"
    a: "Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans. ChatGPT connects to remote MCP servers over HTTPS, so you run the connector's local binary on your machine and expose it over HTTPS - most of them serve Streamable HTTP themselves with `--transport http`, and the stdio-only ones go behind a supergateway bridge. Claude Desktop and Claude Code connect to the same local connector more directly."
  - q: "Do I need to write code to connect Claude to my PSA?"
    a: "No. You install a connector by pasting one sentence into Claude Code or Codex - your agent does the install - or by running a one-line installer for your OS. After that you ask questions in plain English and never type the underlying command yourself; the AI runs it for you. You do enter your PSA's API credentials once."
---

# How do MSPs use Claude and ChatGPT with their PSA?

MSPs install a free **MCP server connector** for their PSA that runs locally on their own machine, then ask plain-English questions in Claude, ChatGPT, Codex, or Copilot. The AI runs the underlying command for them - triage the queue, build a client card, find SLA breaches, run cross-client analytics. You paste one sentence to install, enter your API credentials once, and after that you just ask questions. No code, no dashboards, no exports.

## What it looks like day to day

You ask in plain English; your AI agent runs the command under the hood.

- "Triage my queue and tell me what's breaching SLA today."
- "Give me the whole picture for Acme Corp on one screen."
- "Which clients had a backup that hasn't succeeded in 7 days?"
- "How many SLA breaches per client last quarter?"

You never type the command. The connector keeps a local copy of your data, so cross-client questions come back instantly instead of as hundreds of slow API calls.

## Which AI works with this

- **Claude Desktop** and **Claude Code** connect to the local connector directly - the most common MSP-owner starting points.
- **ChatGPT** (Plus and up) connects to remote MCP servers over HTTPS; you expose the local binary via a bridge.
- **Codex CLI** and **GitHub Copilot** (Agent mode) run the local connector too.

Full per-agent setup is on the [which agent? →](/which-agent/) page.

## Read this next

- Is this safe with client data? [the Trust Center →](/governance/) · [are MCP servers safe for client data? →](/answers/are-mcp-servers-safe-for-msp-client-data/)
- New to the term? [what is an MCP server? →](/what-is-an-mcp-server/) · [what is a Claude Code Skill? →](/answers/what-is-a-claude-code-skill/)
- See it on a real tool: [ConnectWise PSA →](/skills/connectwise-manage/) · [Autotask →](/skills/autotask/) · [HaloPSA →](/skills/halopsa/) · [NinjaOne →](/skills/ninjaone/) · [Datto RMM →](/skills/datto-rmm/)
