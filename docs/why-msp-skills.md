---
layout: default
title: "Why msp-skills? The cross-client AI layer for MSP owners"
description: "Why MSP owners install msp-skills instead of leaning on vendor-native AI: it answers questions the API can't, joins systems instead of reading one, holds one quality bar per connector, complements what your vendors ship, and is free, local, and yours."
permalink: /why/
faqs:
  - q: "Why use msp-skills instead of my PSA or backup vendor's built-in AI?"
    a: "Vendor-native AI is great per-ticket - rewriting a reply, summarizing one ticket. msp-skills is the cross-client, cross-system layer: it joins your PSA, backup, and RMM data and answers questions across your whole book of business that no single vendor API returns in one shot. The two complement each other; nothing gets ripped out."
  - q: "Why is msp-skills faster than a normal MCP server for cross-client questions?"
    a: "Most MCP servers proxy each question into a live API call, so a 90-day, 10-client question becomes hundreds of paginated calls and rate-limit headaches. msp-skills keeps a private, searchable copy of your data on your machine, so that question is one instant local lookup."
  - q: "Is msp-skills really free?"
    a: "Yes. The Skills, CLI binaries, and MCP servers are Apache-2.0 licensed - free to use commercially, free to fork. You pay only for whichever AI agent you already use. There is no per-tech fee and no SaaS subscription."
  - q: "How do I actually get my vendor's API key connected?"
    a: "Use connect-tool. It drives the Chrome you are already logged into, reads the displayed API key (or takes a hidden paste), and writes it straight into the macOS Keychain or Windows Credential Manager without the complete value passing through the agent, so it never lands in a config file. For a new or changed connection it will not report success until a real authenticated read returns your live data. Details at /skills/connect-tool/."
---

# Why msp-skills?

msp-skills is the layer that lets your AI answer questions across your whole MSP - every client, every system - that no single vendor API can return in one shot. It keeps a private, searchable copy of your tools' data on your own machine, joins your PSA with your backup and RMM data, and holds every connector to one verification bar before it ships. It is free, open source, and yours. Here is why MSP owners install it.

## It answers questions your vendor's API can't

Picture QBR prep: you want backup status across 90 days for 10 clients. Through a normal live API that is roughly 900 calls - paginated, rate-limited, and slow enough that you give up and pull a screenshot instead.

msp-skills keeps a **private, searchable copy of your data on your own machine**. That same question is one instant local lookup. No rate limits, no waiting, no exporting three reports into Excel. The first sync happens once; after that it stays current incrementally, and every cross-client question runs against the local copy.

## It joins systems, not just reads one

A connector that only reads one tool answers one tool's questions. The MSP questions that actually matter span systems.

- **Triage** pulls tickets, clients, and assets together so your queue is legible across the whole team, not one tech's head.
- A **client card** assembles tickets, contracts, time, and backup state for one client onto a single screen - on demand, for anyone.
- **Contract burn** catches unpriced work eating margin before it shows up on an invoice.

These are joins across entities and across products. A stateless API wrapper can't do them; a local mirror with full-text search can.

## One quality bar, every connector

Every connector passes the same mechanical verification before it ships:

- **Build** - the CLI and MCP server compile and run on macOS, Linux, and Windows.
- **Command-surface vs docs** - every command and flag the docs claim actually exists in the real binary.
- **Claims check + install dry-run** - the install path resolves cleanly and the documented behavior matches the binary.

That is the floor for every connector, the day it ships. On top of that, each one carries a **Live-verified** badge that marks whether a real MSP has confirmed it against a live production tenant. Until then it reads **Awaiting live verification** - not a defect, an open invitation. If you run it against your tenant and it works, [tell us](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml) and you become the one who flipped the badge.

## It complements what your vendors ship

Your PSA and backup vendors are adding AI, and that is good. Vendor-native AI is excellent for per-ticket work: rewriting a reply, summarizing one ticket, flagging sentiment. It lives inside one product and answers one product's questions.

msp-skills is the **cross-client, cross-system layer that sits above** all of them. It does the thing no single-vendor AI can: ask across thousands of tickets, across every client, across your PSA and your backup tool at once. You don't choose between them. Nothing gets ripped out. You add the layer your vendors structurally can't ship, because it has to span vendors.

### My vendor already ships an MCP server. Why add this one?

Keep it - it's good at in-product work. An MSP Skills connector is complementary: it keeps a private local copy of your data so cross-client, cross-system questions come back in one ask instead of hundreds of rate-limited calls.

| | A typical MCP server | An MSP Skills connector |
| --- | --- | --- |
| Where answers come from | A live API call per question | A private local copy of your data, plus the live API |
| Cross-client questions | One client at a time, paginated | The whole book of business in one ask |
| What the AI reads | Raw records streamed into the chat | Query results - the answer, not the bulk data |
| Systems per question | One (the vendor's own product) | Many - join your PSA, backup, RMM, and M365 |
| Where it runs | Often a hosted cloud service | Your own machine - nothing leaves your network |

## Free, local, yours

- **Free.** Apache-2.0 licensed. Free to use commercially, free to fork. No per-tech fee.
- **Local.** The CLI and MCP server are binaries on your machine; the data copy sits under your user account. Your AI sees query results, not raw bulk data, and no data leaves your network.
- **Yours.** Open source, no proprietary plugin format, no SaaS subscription that ties you to one AI. Use one connector or all of them, with one agent or several.

## The place to get MSP connectors

The vision is simple: **one install pattern, every MSP tool**. HaloPSA and Servosity are live today; 30+ connectors are coming through a build pipeline, and we build the next ones live with real MSPs in free weekly [Build Sessions](https://compoundingteams.com/build-sessions). Bring the system you want covered and watch it built against a real tenant.

Two honest notes on what the badges mean:

- Most MCP servers proxy each question straight into a live API call - fine for one record, painful for cross-client questions. msp-skills caches into a local mirror instead, which is why the cross-client questions are fast.
- **Awaiting live verification** is a verified connector that simply hasn't been driven against a real tenant by an MSP yet. It already passed every mechanical gate. The badge is an invitation to be the first - not a warning.

Ready? [See what's in the box on the homepage →](/) or [pick your agent →](/which-agent/). New to the term itself? [What is an MCP server? →](/what-is-an-mcp-server/)
