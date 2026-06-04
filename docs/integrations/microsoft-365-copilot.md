---
layout: default
title: "HaloPSA + Servosity in Microsoft 365 Copilot / Copilot Studio"
description: "How to use HaloPSA and Servosity in Microsoft 365 Copilot and Copilot Studio via MCP. Honest guide: remote Streamable-HTTP only, needs hosting + a Copilot Studio license + tenant admin. Free MSP Skills."
permalink: /integrations/microsoft-365-copilot/
faqs:
  - q: "Does Microsoft 365 Copilot support HaloPSA?"
    a: "Yes, but only via the remote path. Microsoft 365 Copilot and Copilot Studio consume MCP over remote Streamable-HTTP - they do not launch local binaries. You host MSP Skills' HaloPSA MCP server over HTTPS and wire it in through Copilot Studio or a declarative agent. A Copilot Studio license and tenant admin are required."
  - q: "Does Microsoft Copilot support MCP?"
    a: "It depends which Copilot. GitHub Copilot (VS Code) runs local MCP servers today. Microsoft 365 Copilot, Copilot Studio, and Security Copilot support MCP over remote HTTPS only. The free consumer Copilot (copilot.microsoft.com, Windows Copilot) does not support custom MCP servers at all."
  - q: "Can I point Microsoft 365 Copilot at a local MCP binary?"
    a: "No. There is no local-stdio path anywhere in the Microsoft 365 Copilot family. You must host the MCP server as a public HTTPS (Streamable-HTTP) endpoint first, then connect it via Copilot Studio (Server URL) or the Microsoft 365 Agents Toolkit."
  - q: "What's the easiest Microsoft path to use HaloPSA with AI?"
    a: "GitHub Copilot in VS Code - it runs MSP Skills' local binary today with no hosting or license. Microsoft 365 Copilot is the broader business assistant but requires hosting, a Copilot Studio license, and tenant admin, so it is not self-serve."
---

# HaloPSA and Servosity in Microsoft 365 Copilot / Copilot Studio

Microsoft 365 Copilot is the Copilot most MSPs and their clients actually have - it's bundled into the Microsoft 365 stack you already resell. So it's worth knowing exactly how MSP Skills connects to it.

**Honest heads-up first.** Microsoft 365 Copilot, Copilot Studio, and Security Copilot consume MCP over **remote Streamable-HTTP only**. There is **no local-stdio path** - the `halopsa-mcp` / `servosity-mcp` binary you install on your laptop is not enough on its own. You also need a **Copilot Studio license** and a **tenant admin** to enable it. This is a build-and-host task, not a 60-second install.

**If that's a blocker:** [GitHub Copilot in VS Code](/integrations/github-copilot/) is the Microsoft surface that runs the local binary today, free, no hosting. Use it if you just want MSP Skills working inside a Microsoft tool quickly.

## What you need

- A **Microsoft 365 Copilot** license, plus a **Copilot Studio** license (for the low-code route)
- **Tenant admin** access (to enable Copilot extensibility / custom app upload)
- A place to **host** the MCP server over HTTPS (a small VM, container, or tunnel)
- A **HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token**

## Step 1 - Install MSP Skills binaries

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

## Step 2 - Host the MCP server over HTTPS

The MSP Skills binaries serve the Streamable-HTTP transport Copilot expects. Run in HTTP mode with credentials in the environment:

```bash
HALOPSA_TENANT=<tenant> \
HALOPSA_CLIENT_ID=<id> \
HALOPSA_CLIENT_SECRET=<secret> \
halopsa-mcp --transport http --addr :7777
```

Then expose `http://localhost:7777` as a public **HTTPS** URL via Cloudflare Tunnel, ngrok (HTTPS), or your own reverse proxy. (Repeat for `servosity-mcp` on another port.) **Treat the URL as a credential** - gate it behind SSO / Cloudflare Access; never expose it bare on the internet.

For team / production use, host MSP Skills on a server inside your network and front it with an SSO-gated tunnel so only authenticated users reach it.

## Step 3a - Wire it in Copilot Studio (lowest-code)

In your Copilot Studio agent:

1. Open **Tools → Add a tool → Model Context Protocol**.
2. Enter a **Server name**, the **Server URL** (your HTTPS endpoint), and auth (**OAuth 2.0** or **API key**).
3. Copilot Studio builds the Power Platform custom connector behind the scenes. **Generative orchestration must be on.**
4. Publish the agent into Microsoft 365 Copilot.

Source: [Microsoft Learn - Extend your agent with MCP](https://learn.microsoft.com/en-us/microsoft-copilot-studio/agent-extend-action-mcp).

## Step 3b - Or build a declarative agent (dev-ish)

Use the **Microsoft 365 Agents Toolkit** in VS Code: **Add an Action → Start with an MCP Server**, point at the remote URL, configure OAuth, then provision / sideload. Requires admin-enabled **Custom App Upload** + **Copilot Access**.

Source: [Microsoft Learn - Build MCP plugins for Microsoft 365 Copilot](https://learn.microsoft.com/en-us/microsoft-365/copilot/extensibility/build-mcp-plugins).

## Step 4 - Ask a real question

Once published, in Microsoft 365 Copilot:

- *"Use the HaloPSA agent: triage what needs attention across all clients today."*
- *"Use the Servosity agent: show me stale backups this week."*

## Cost

- **MSP Skills:** Free (Apache-2.0).
- **Microsoft 365 Copilot + Copilot Studio:** your Microsoft licensing (see Microsoft's pricing).
- **Hosting:** Cloudflare Tunnel is free; a small VM/container is a few dollars a month.
- **HaloPSA / Servosity access:** your existing license / partner agreement.

## Security

- The HTTPS endpoint is a key to your MCP server. Authenticate it (OAuth 2.0) and gate it behind SSO.
- HaloPSA + Servosity credentials live in the host's environment, never transmitted to MSP Skills or to Microsoft.
- Apply your tenant's DLP / VNet governance to the connector.

## Note: free / consumer Copilot

The free consumer Copilot (copilot.microsoft.com, Windows Copilot) does **not** support user-supplied MCP servers. There is no MSP Skills path for it.

## What's next

- **Want it working today without hosting?** Use [GitHub Copilot](/integrations/github-copilot/) or [Claude Desktop](/integrations/claude-desktop/).
- **Bring it to a Build Session.** Work your tenant + your hardest cross-client question live with the MSP cohort: [Build Sessions](https://compoundingteams.com/build-sessions).

[← Back to main install](/#install-in-60-seconds)
