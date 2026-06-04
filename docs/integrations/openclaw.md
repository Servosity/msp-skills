---
layout: default
title: "HaloPSA + Servosity in OpenClaw"
description: "Install MSP Skills (HaloPSA tickets + Servosity backups) in OpenClaw, the browser-controlled AI agent with native Skill + MCP support. Free, open source."
permalink: /integrations/openclaw/
faqs:
  - q: "Does OpenClaw support HaloPSA?"
    a: "Yes. OpenClaw has native Skill support (a SKILL.md superset of Anthropic's spec) and native MCP, so it runs MSP Skills' HaloPSA connector either as a Skill or as an MCP server."
  - q: "How do I install MSP Skills in OpenClaw?"
    a: "Run 'openclaw skills install git:Servosity/msp-skills/skills/halopsa@main' to install the Skill, or register the binary with 'openclaw mcp set halopsa'. OpenClaw reads the skill's openclaw frontmatter and installs the required CLI; run 'openclaw doctor' to confirm."
  - q: "Is OpenClaw generally available?"
    a: "OpenClaw is GA as of May 2026 with a public skill registry (ClawHub). The MSP Skills frontmatter is pre-wired; if the monorepo subdirectory git: path doesn't resolve in your build, clone the repo locally and install from the path."
---

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
- **Full per-tool wire-up:** [Which AI agent?](/which-agent/)

[← Back to main install](/#install-in-60-seconds)
