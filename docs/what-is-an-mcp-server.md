---
layout: default
title: "What is an MCP server? A plain-language answer for MSPs"
description: "An MCP server is a small program that connects your AI to a real system like your PSA, safely, with your credentials, on your machine. Plain-language answer for MSP owners: what it is, how it differs from a chatbot integration, and what you can do with one."
permalink: /what-is-an-mcp-server/
faqs:
  - q: "Is an MCP server the same as a ChatGPT connector?"
    a: "Yes - same thing, different name. What this site calls an MCP server is what ChatGPT calls an app or connector, Claude on the web calls a connector, Microsoft Copilot calls a connector, and Claude Code calls a Skill. They all use the Model Context Protocol underneath."
  - q: "Do I need to know how to code to use an MCP server?"
    a: "No. You install one by pasting a single sentence into Claude Code or Codex (your agent does the rest) or by running a one-line installer. After that you ask questions in plain English - you never type the underlying command yourself; your AI agent runs it for you."
  - q: "Is my data safe with an MCP server?"
    a: "With these MCP servers, yes. They run locally on your own machine with your own credentials. The data copy stays under your user account, your AI sees only query results rather than raw bulk data, and nothing is transmitted to our servers - there are none."
  - q: "What MCP servers exist for MSP tools?"
    a: "HaloPSA and Servosity are live today, with more PSA, RMM, backup, and M365 connectors being built every week. Each one connects the tool to the AI you already use and keeps a local copy of its data so cross-client questions are fast."
---

# What is an MCP server?

An **MCP server** is a small program that connects your AI to a real system - like your PSA, RMM, or backup tool - safely, with your own credentials, running on your own machine. It lets you ask your AI a plain-English question about that system ("which clients had backup failures last quarter?") and get a real answer back, instead of the AI guessing or sending you off to a dashboard.

{% include vocab-bridge.html %}

## How it differs from a chatbot integration

A plain chatbot only knows what it was trained on plus whatever you paste into the chat. It can't see your live HaloPSA queue or your backup fleet.

An MCP server changes that. It is a connector that lets your AI **call a real system on your behalf** - read a ticket, list stale backups, pull a client's contract burn - using credentials you supply. The AI asks the MCP server, the MCP server talks to the tool, and a real answer comes back. Two things make these particular MCP servers different from a typical one:

- They **run on your machine**, not in someone else's cloud. Your data and credentials stay with you.
- They keep a **local searchable copy** of your tool's data, so cross-client questions ("across all 47 clients...") are one instant lookup instead of hundreds of slow API calls.

## What an MSP can do with one

You ask in plain English; your AI agent runs the command for you under the hood.

{% include instead-of.html
   instead="clicking through 47 client portals to find stale backups"
   say="Which clients have a backup that hasn't succeeded in 7 days?"
   cli="servosity-cli stale-backups --days 7" %}

{% include instead-of.html
   instead="opening four HaloPSA screens to brief yourself before a client call"
   say="Give me the whole picture for Acme Corp on one screen."
   cli="halopsa-cli client card 'Acme Corp'" %}

{% include instead-of.html
   instead="exporting tickets and pivoting them in Excel for the QBR"
   say="How many SLA breaches did we have per client last quarter?"
   cli="halopsa-cli sla breaching --within 24h" %}

## Is an MCP server the same as a ChatGPT connector?

Yes. ChatGPT calls them **apps** or **connectors**; Claude on the web and Microsoft Copilot call them **connectors**; Claude Code calls them **Skills**. They are all the same idea built on the same open standard - the Model Context Protocol. This site says "MCP server" because that is the precise name, but if your AI shows you "connectors" or "apps," that is where these go.

## Do I need to know how to code to use an MCP server?

No. There are two install paths and neither requires writing code:

- **Paste one sentence** into Claude Code or Codex and your AI reads the instructions, installs the binary, and walks you through authentication.
- **Run a one-line installer** for your OS (a single bash or PowerShell command).

After that you ask questions in plain English. You never type the underlying command - your AI agent runs it for you. You will need to paste in API credentials your PSA or backup tool gives you, which the tool's own settings page provides.

## Is my data safe with an MCP server?

With these MCP servers, yes. They run **locally** - the binary lives on your laptop, the data copy sits in a directory under your user account, and your credentials come from your own environment or your agent's config. Your AI sees the result of a query, not raw bulk data, and nothing is transmitted to our servers (there are none). Each connector also ships a permission matrix that tags every command read, write, or destructive, so you can scope what an agent is allowed to do.

## What MCP servers exist for MSP tools?

HaloPSA and Servosity are live today, and more PSA, RMM, backup, and M365 connectors are being built every week in free weekly [Build Sessions](https://compoundingteams.com/build-sessions). Each one connects the tool to the AI you already use and keeps a local copy of its data so cross-client questions are fast.

## How to get one

- [See what's available and install it on the homepage →](/)
- [Read why MSP owners use these connectors →](/why/)
- Per-tool pages: [HaloPSA →](/skills/halopsa/) · [Servosity →](/skills/servosity/)
- Not sure which AI you have? [Which agent? →](/which-agent/)
