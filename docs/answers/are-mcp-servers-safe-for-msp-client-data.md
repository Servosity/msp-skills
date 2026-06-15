---
layout: default
title: "Are MCP servers safe for MSP client data? | MSP Skills"
description: "Whether MCP servers are safe for MSP client data depends on where they run. MSP Skills connectors run on your own machine with your own credentials, send nothing to outside servers, tier every command read/write/destructive, and let your own tenant scope what the API key can reach. Plain-language answer for MSP owners."
permalink: /answers/are-mcp-servers-safe-for-msp-client-data/
faqs:
  - q: "Are MCP servers safe for MSP client data?"
    a: "It depends on where the MCP server runs. A hosted MCP server sends your data through someone else's cloud. MSP Skills connectors run locally on your own machine with your own credentials, send nothing to outside servers, and only ever return query results to the AI - not raw bulk data. That local, no-telemetry design is what makes them safe for client data."
  - q: "Does using an MCP server send my client data to a third party?"
    a: "With MSP Skills connectors, no. The binaries run on your machine and talk only to your vendor's API using your credentials. There is no server of ours in the path, no telemetry, and any local data copy stays under your user account. Your AI agent sees the answer to a question, not a dump of your database."
  - q: "How do I stop an AI agent from doing something destructive in my PSA?"
    a: "Two controls. Every command is tiered read, write, or destructive, and the recommended policy requires a human for destructive actions and a --dry-run preview plus your approval for writes. The strongest control is server-side: scope the API key in your own tenant so the connector can only reach what you granted, regardless of what the agent tries."
---

# Are MCP servers safe for MSP client data?

It depends on where the MCP server runs. A hosted one routes your data through someone else's cloud. **MSP Skills connectors run locally on your own machine** with your own credentials, send nothing to outside servers, and return only query results to the AI - never a raw dump of client data. Every command is tiered read / write / destructive, and your own tenant scopes what the API key can reach. That design is what makes them safe.

## Why "where it runs" is the whole question

An MCP server is a connector that lets your AI call a real system on your behalf. The safety question is almost entirely about **location**:

- A **hosted** MCP server puts a third party between your AI and your data. Your client data flows through their infrastructure.
- A **local** MCP server, like every MSP Skills connector, is a binary on your laptop. It talks straight to your vendor's API with your credentials. There is no middleman, no telemetry, and no servers of ours in the path.

## The two layers of control

1. **Command tiers.** Reads run autonomously; writes require a `--dry-run` preview and your approval; destructive actions require a human. Each connector ships this policy in its `governance.md`.
2. **Server-side scoping.** Issue the API key inside your own PSA or backup tenant and scope it there. The connector can never exceed what your tenant granted - this control does not depend on the agent behaving.

## Read this next

- The full safety model: [the Trust Center →](/governance/)
- The plain-language basics: [what is an MCP server? →](/what-is-an-mcp-server/)
- How this compares to vendor AI: [MCP server vs vendor built-in AI →](/answers/mcp-server-vs-vendor-built-in-ai/)
- How MSPs actually use this: [using Claude and ChatGPT with your PSA →](/answers/how-msps-use-claude-and-chatgpt-with-their-psa/)
- See it on a real tool: [ConnectWise PSA →](/skills/connectwise-manage/) · [Autotask →](/skills/autotask/) · [HaloPSA →](/skills/halopsa/) · [NinjaOne →](/skills/ninjaone/) · [Datto RMM →](/skills/datto-rmm/)
