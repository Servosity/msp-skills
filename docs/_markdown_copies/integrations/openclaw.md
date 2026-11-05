> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/integrations/openclaw/

# HaloPSA and Servosity in OpenClaw

[OpenClaw](https://docs.openclaw.ai) is a browser-controlled AI agent with native Skill support (a `SKILL.md` superset of Anthropic's spec) and native MCP. Like Hermes, it's skill-native - it reads MSP Skills' `SKILL.md` directly **and** speaks MCP.

## What you need

- **OpenClaw** installed ([install docs](https://docs.openclaw.ai/install))
- A **HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token**

## Step 1 - Install OpenClaw (one time)

```bash
# macOS / Linux / WSL2
curl -fsSL https://openclaw.ai/install.sh | bash

# Windows (PowerShell)
iwr -useb https://openclaw.ai/install.ps1 | iex
```

## Path A - install as a Skill (recommended)

```bash
openclaw skills install git:Servosity/msp-skills/skills/halopsa@main
openclaw skills install git:Servosity/msp-skills/skills/servosity@main
```

OpenClaw reads each skill's `SKILL.md`, sees the `metadata.openclaw.requires.bins` block (already in the frontmatter), and installs the required CLI. Confirm with:

```bash
openclaw doctor
```

## Path B - register as an MCP server

```bash
openclaw mcp set halopsa '{"command":"halopsa-mcp"}'
openclaw mcp set servosity '{"command":"servosity-mcp"}'
```

Set the same env vars the CLI needs. See the [OpenClaw MCP CLI docs](https://docs.openclaw.ai/cli/mcp) for the canonical command shape.

## Step 2 - Ask a real question

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

## Note on the subdirectory install path

OpenClaw's monorepo subdirectory `git:` syntax is documented in the ClawHub skill-format spec. If `git:Servosity/msp-skills/skills/halopsa@main` doesn't resolve in your build, clone the repo locally and run:

```bash
openclaw skills install ./msp-skills/skills/halopsa
```

## What's next

- **Try a real workflow** at a free [Build Session](https://compoundingteams.com/build-sessions).
- **Full per-tool wire-up:** [Which AI agent?](https://msp-skills.compoundingteams.com/which-agent.md)

[← Back to main install](https://msp-skills.compoundingteams.com/#install-in-60-seconds)
