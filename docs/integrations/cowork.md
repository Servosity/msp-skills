---
layout: default
title: "HaloPSA + Servosity in Claude Cowork (Anthropic desktop agent)"
description: "Install MSP Skills in Claude Cowork by pasting one prompt. Cowork runs shell on your behalf - no terminal commands you type yourself, no JSON editing. For MSP business owners on Anthropic's GA desktop agent."
permalink: /integrations/cowork/
faqs:
  - q: "Does Claude Cowork support HaloPSA?"
    a: "Yes. Paste a one-paragraph prompt into Cowork and it detects your shell, runs the installer, walks authentication, and confirms. Or use Cowork's Settings > Customize > Connectors UI to add the HaloPSA MCP server as a remote connector. Either path works."
  - q: "What is Claude Cowork?"
    a: "Anthropic's desktop agent product, GA'd March 2026. Sits between Claude Desktop (chat UI, no shell) and Claude Code (terminal-native): Cowork runs shell commands on your behalf when you ask it to and exposes a Settings > Connectors UI for MCP servers."
  - q: "Does Cowork support MCP?"
    a: "Yes - both as a shell-executing agent (paste a prompt that runs the install script) and as an MCP host via Settings > Customize > Connectors. The Composio-style 'connect via URL' pattern is documented for remote/HTTPS MCP servers."
  - q: "Do I need to pay for Cowork?"
    a: "Cowork itself follows Anthropic's standard Claude pricing - you pay for Claude usage the same way you would for Claude Desktop or Claude Code. MSP Skills itself is free."
---

# HaloPSA and Servosity in Claude Cowork

Claude Cowork is Anthropic's desktop agent product (GA'd March 2026). It sits between **Claude Desktop** (chat UI, no shell) and **Claude Code** (terminal-native): Cowork **runs shell on your behalf** when you ask it to. That makes the MSP Skills install dead simple - you paste one prompt and Cowork does the rest.

## Install in 30 seconds

Paste this into Cowork to install the **HaloPSA** Skill + MCP server:

> Install the HaloPSA Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex`. Then authenticate with `halopsa-cli auth login` (Configuration > Integrations > Halo PSA API in your tenant gives you the credentials) and run `halopsa-cli --help` to explore.

And for the **Servosity** skill:

> Install the Servosity Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex`. Then authenticate with `SERVOSITY_MSP_TOKEN=<your-partner-token> servosity-cli doctor` and run `servosity-cli --help` to explore.

Cowork detects your shell, runs the installer, walks authentication, and confirms. No JSON editing, no Connector configuration, no terminal commands you type yourself.

## Alternative: MCP Connector path

Cowork also supports remote/HTTPS MCP servers via its Settings > Customize > Connectors UI - that's the Composio pattern documented for HubSpot and similar integrations. **For MSP Skills' local stdio binaries, the paste-prompt above is the simpler path.** If you want to expose the MCP server over HTTPS instead (e.g. for team-wide access from a shared host), follow the remote-agent section of the skill's `mcp-install.md`: most binaries serve Streamable HTTP themselves with `--transport http` at the `/mcp` path, and the stdio-only ones go behind a `supergateway` bridge.

## Use the skill

In Cowork:

- *"Use halopsa: triage what needs attention across all clients today, focused on SLA breaches in the next 24 hours."*
- *"Use servosity: show me stale backups across all clients, grouped by backup engine."*
- *"Use halopsa: build a client card for Acme Corp, then cross-reference any open Servosity backup issues for them."*

Cowork reads the SKILL.md (it sits at `~/.claude/skills/halopsa/SKILL.md` after install) and runs the right commands.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions).
- **Switching agents?** Same skills work in [Claude Desktop](/integrations/claude-desktop/), [Claude Code](/integrations/claude-code/), [Codex CLI](/integrations/codex/), and [ChatGPT (Plus/Pro+)](/integrations/chatgpt/) - no reinstall needed.
- **Want a different platform?** [Request a Skill →](/requesting-a-skill/).

[← Back to main install](/#install-in-60-seconds)
