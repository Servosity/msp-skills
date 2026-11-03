> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/cowork/

# HaloPSA and Servosity in Claude Cowork

Claude Cowork is Anthropic's desktop agent product (GA'd March 2026). It sits between **Claude Desktop** (chat UI, no shell) and **Claude Code** (terminal-native): Cowork **runs shell on your behalf** when you ask it to. That makes the MSP Skills install dead simple - you paste one prompt and Cowork does the rest.

## Install in 30 seconds

Paste this into Cowork to install the **HaloPSA** Skill + MCP server:

> Install the HaloPSA Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex`. Then authenticate with `halopsa-cli auth login` (Configuration > Integrations > Halo PSA API in your tenant gives you the credentials) and run `halopsa-cli --help` to explore.

And for the **Servosity** skill:

> Install the Servosity Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex`. Then authenticate with `SERVOSITY_MSP_TOKEN=<your-partner-token> servosity-cli doctor` and run `servosity-cli --help` to explore.

Cowork detects your shell, runs the installer, walks authentication, and confirms. No JSON editing, no Connector configuration, no terminal commands you type yourself.

## Alternative: MCP Connector path

Cowork also supports remote/HTTPS MCP servers via its Settings > Customize > Connectors UI - that's the Composio pattern documented for HubSpot and similar integrations. **MSP Skills connectors run locally over stdio, so use the paste-prompt above.** Don't run a connector in HTTP mode or put it behind a tunnel to use the Connectors URL path: our [SECURITY.md](https://github.com/Servosity/msp-skills/blob/main/SECURITY.md#handling-credentials-safely) says not to until each connector is reprinted.

## Use the skill

In Cowork:

- *"Use halopsa: triage what needs attention across all clients today, focused on SLA breaches in the next 24 hours."*
- *"Use servosity: show me stale backups across all clients, grouped by backup engine."*
- *"Use halopsa: build a client card for Acme Corp, then cross-reference any open Servosity backup issues for them."*

Cowork reads the SKILL.md (it sits at `~/.claude/skills/halopsa/SKILL.md` after install) and runs the right commands.

## What's next

- **Try a real workflow.** Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions).
- **Switching agents?** Same skills work in [Claude Desktop](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md), [Claude Code](https://msp-skills.compoundingteams.com/integrations/claude-code.md), [Codex CLI](https://msp-skills.compoundingteams.com/integrations/codex.md), and [the ChatGPT desktop app](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) - no reinstall needed.
- **Want a different platform?** [Request a Skill →](https://msp-skills.compoundingteams.com/requesting-a-skill.md).

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
