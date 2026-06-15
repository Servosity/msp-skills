---
layout: default
title: "What is a Claude Code Skill? A plain answer for MSPs | MSP Skills"
description: "A Claude Code Skill is a set of instructions and a tool that an AI agent reads to do a real job - like answering questions about your PSA. It's the same idea most AIs call an MCP server or connector. Plain-language answer for MSP owners, with how to install one and use it across your tools."
permalink: /answers/what-is-a-claude-code-skill/
faqs:
  - q: "What is a Claude Code Skill?"
    a: "A Claude Code Skill is a package of instructions plus a tool that an AI agent reads to do a real job - like connecting to your PSA and answering questions about it. It's the same idea most AI surfaces call an MCP server or connector. Claude Code reads the Skill directly; other agents load the same package as an MCP server. One install gives you both."
  - q: "Is a Claude Code Skill the same as an MCP server?"
    a: "Effectively yes - they're two interfaces to the same thing. A Skill is what Claude Code and Codex read directly (a SKILL.md plus a local binary). An MCP server is what Claude Desktop, ChatGPT, Cursor, and Copilot load over the Model Context Protocol. MSP Skills connectors ship both in one install, so you use whichever your AI speaks."
  - q: "Do I need Claude Code to use a Claude Code Skill?"
    a: "No. Claude Code reads the Skill directly, but the same package is also an MCP server that works with Claude Desktop, ChatGPT, Codex, Cursor, Copilot, and any MCP-capable agent. You install once and use the AI you already have."
---

# What is a Claude Code Skill?

A **Claude Code Skill** is a package of instructions plus a tool that an AI agent reads to do a real job - like connecting to your PSA and answering questions about it. It's the same idea most AI surfaces call an **MCP server** or **connector**, just the name Claude Code uses. Claude Code reads the Skill directly; other agents load the very same package as an MCP server. One install gives you both, so you use whichever your AI speaks.

## The one-thing-many-names map

{% include vocab-bridge.html %}

If your AI shows you "connectors" or "apps," that's where these go. If you use Claude Code, it's a "Skill." Underneath, it's all the **Model Context Protocol** - one open standard.

## Why an MSP cares

A plain chatbot only knows what it was trained on plus what you paste in. A Skill (or MCP server) lets your AI **call your real systems** - read a ticket, list stale backups, pull a client's contract burn - using credentials you supply, on your own machine. That's the difference between an AI that guesses and one that answers from your live data.

These particular connectors also keep a **local searchable copy** of your tool's data, so cross-client questions ("across all 47 clients...") are one instant lookup instead of hundreds of slow API calls.

## How you get one

- **Paste one sentence** into Claude Code or Codex - your agent reads the Skill and installs the binary.
- **Run a one-line installer** for your OS (bash or PowerShell).

Neither path requires code. After that you ask questions in plain English and the AI runs the command for you.

## Read this next

- The fuller glossary: [what is an MCP server? →](/what-is-an-mcp-server/)
- How MSPs use it daily: [using Claude and ChatGPT with your PSA →](/answers/how-msps-use-claude-and-chatgpt-with-their-psa/)
- Vendor AI vs this: [MCP server vs vendor built-in AI →](/answers/mcp-server-vs-vendor-built-in-ai/)
- Is it safe? [are MCP servers safe for client data? →](/answers/are-mcp-servers-safe-for-msp-client-data/) · [the Trust Center →](/governance/)
- See it on a real tool: [ConnectWise PSA →](/skills/connectwise-manage/) · [Autotask →](/skills/autotask/) · [HaloPSA →](/skills/halopsa/) · [NinjaOne →](/skills/ninjaone/) · [Datto RMM →](/skills/datto-rmm/)
