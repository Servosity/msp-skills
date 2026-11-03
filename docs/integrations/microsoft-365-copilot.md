---
layout: default
title: "HaloPSA + Servosity in Microsoft 365 Copilot / Copilot Studio"
description: "Microsoft 365 Copilot and Copilot Studio reach MCP servers only at a remote web address. MSP Skills connectors run locally and their HTTP mode is paused until each one is reprinted, so Microsoft 365 Copilot is not supported yet. Use a hosted gateway, or GitHub Copilot in VS Code locally."
permalink: /integrations/microsoft-365-copilot/
faqs:
  - q: "Does Microsoft 365 Copilot support HaloPSA?"
    a: "Not through MSP Skills yet. Microsoft 365 Copilot and Copilot Studio consume MCP over remote Streamable-HTTP only - they do not launch local binaries - and MSP Skills' HTTP mode is paused until each connector is reprinted. If your team works in Microsoft 365 Copilot, a hosted gateway is the better pick; check that your AI app is on the gateway's setup list."
  - q: "Does Microsoft Copilot support MCP?"
    a: "It depends which Copilot. GitHub Copilot (VS Code) runs local MCP servers today. Microsoft 365 Copilot, Copilot Studio, and Security Copilot support MCP over remote HTTPS only. The free consumer Copilot (copilot.microsoft.com, Windows Copilot) does not support custom MCP servers at all."
  - q: "Can I point Microsoft 365 Copilot at a local MCP binary?"
    a: "No. There is no local-stdio path anywhere in the Microsoft 365 Copilot family. It needs a remote (HTTPS) server, which MSP Skills does not support yet."
  - q: "What's the easiest Microsoft path to use HaloPSA with AI?"
    a: "GitHub Copilot in VS Code - it runs MSP Skills' local binary today with no hosting or license."
---

# HaloPSA and Servosity in Microsoft 365 Copilot / Copilot Studio

Microsoft 365 Copilot is the Copilot most MSPs and their clients actually have - it's bundled into the Microsoft 365 stack you already resell. So it's worth knowing exactly where MSP Skills stands with it.

**Not supported by MSP Skills yet.** Microsoft 365 Copilot, Copilot Studio, and Security Copilot consume MCP over **remote Streamable-HTTP only**. There is **no local-stdio path**: they reach an MCP server at a web address, not the `halopsa-mcp` / `servosity-mcp` binary on your laptop. Reaching them would mean running a connector in HTTP mode, and our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says not to do that until it is reprinted. Don't put a connector behind a tunnel or reverse proxy to reach Copilot either.

## What to use instead

- **Your team works in Microsoft 365 Copilot?** A [hosted gateway](/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick: it gives you that web address on day one. Check that your AI app is on the gateway's setup list (Conduit documents Microsoft 365 Copilot, [getting started](https://conduit.wyre.ai/docs/getting-started/)).
- **Want MSP Skills inside a Microsoft tool today?** [GitHub Copilot in VS Code](/integrations/github-copilot/) runs the local binary, free, no hosting.
- **Prefer a chat app?** [Claude Desktop](/integrations/claude-desktop/) and [the ChatGPT desktop app](/integrations/chatgpt/) run the same local connector.

## Note: free / consumer Copilot

The free consumer Copilot (copilot.microsoft.com, Windows Copilot) does **not** support user-supplied MCP servers. There is no MSP Skills path for it.

## What's next

- **Bring it to a Build Session.** Work your tenant + your hardest cross-client question live with the MSP cohort: [Build Sessions](https://compoundingteams.com/build-sessions).

[← Back to main install](/#install-in-60-seconds)
