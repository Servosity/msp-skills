---
layout: default
title: "HaloPSA + Servosity in OpenAI Codex CLI"
description: "Install MSP Skills in OpenAI Codex CLI: paste one sentence and your agent reads the SKILL.md, runs the install, walks authentication. Same skills, OpenAI-side."
permalink: /integrations/codex/
faqs:
  - q: "Does Codex CLI support HaloPSA?"
    a: "Yes. Codex CLI reads SKILL.md files the same way Claude Code does. Paste 'Set up the HaloPSA skill from https://github.com/servosity/msp-skills' and Codex does the install. Also supports MCP via ~/.codex/config.toml."
  - q: "What is OpenAI Codex CLI?"
    a: "OpenAI's command-line agent - a terminal-native AI similar to Anthropic's Claude Code, but using OpenAI's models (GPT-5/4 family). Supports Claude-Code-style Skills AND the open MCP protocol. Available at developers.openai.com/codex."
  - q: "Can Codex CLI use the same MSP Skills as Claude Code?"
    a: "Yes - these skills are agent-agnostic. The CLI binary, the MCP server, the SKILL.md format, and the install path are identical. Switch between Claude Code, Codex CLI, and Claude Desktop without reinstalling MSP Skills."
  - q: "Do I need to pay OpenAI to use Codex CLI?"
    a: "Codex CLI usage is billed against your OpenAI API account (token-based) or your ChatGPT subscription, depending on the model. MSP Skills is free regardless."
---

# HaloPSA and Servosity in OpenAI Codex CLI

Codex CLI is OpenAI's command-line agent - a terminal-native AI that reads `SKILL.md` files (the open Agent Skills spec format) AND speaks MCP natively. For the same audience as Claude Code: a **technical-leaning MSP owner** or a **senior tech**.

If you use Claude Code already, the install is identical - same skills, same SKILL.md, same MCP servers. Switching agents costs nothing.

## Install in 30 seconds

Paste this into Codex CLI:

> Set up the **HaloPSA** skill from https://github.com/servosity/msp-skills - read `skills/halopsa/SKILL.md`, run its install steps, then run `halopsa-cli --version` to confirm. Walk me through authentication.

For both skills:

> Set up both the **halopsa** and **servosity** skills from https://github.com/servosity/msp-skills - read each skill's SKILL.md, run their install steps, then run `halopsa-cli --version` and `servosity-cli doctor` to confirm.

Codex reads each SKILL.md, runs the installer (drops the CLI + MCP binary on your PATH), prompts you for credentials, runs `doctor` to verify.

## Manual install + MCP registration

If you prefer explicit config:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

Then edit `~/.codex/config.toml` (global) or `.codex/config.toml` (project) to register the MCP servers:

```toml
[mcp_servers.halopsa]
command = "halopsa-mcp"
env = { HALOPSA_TENANT = "<your-tenant>", HALOPSA_CLIENT_ID = "<id>", HALOPSA_CLIENT_SECRET = "<secret>" }

[mcp_servers.servosity]
command = "servosity-mcp"
env = { SERVOSITY_MSP_TOKEN = "<token>" }
```

Restart Codex CLI. Both MCP servers connect on startup.

Codex supports stdio + Streamable HTTP and runs concurrent read-only tools when advertised - useful for cross-client / cross-engine analytics that hit multiple MSP Skills tools at once.

[Source: developers.openai.com/codex/mcp](https://developers.openai.com/codex/mcp)

## Use the skill

In Codex CLI:

- *"Use halopsa: triage what needs attention across all clients today, focused on SLA breaches in the next 24 hours."*
- *"Use servosity: show me stale backups across all clients, grouped by backup engine."*

Codex knows the command surface (it just read the SKILL.md), runs the right command with the right flags, and pipes the JSON output into reasoning.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions).
- **Switching agents?** Same skills work in [Claude Code](/integrations/claude-code/), [Claude Desktop](/integrations/claude-desktop/), and [ChatGPT (Plus/Pro+)](/integrations/chatgpt/) - no reinstall.
- **Want a different platform?** [Request a Skill →](/requesting-a-skill/).

[← Back to main install](/#install-in-60-seconds)
