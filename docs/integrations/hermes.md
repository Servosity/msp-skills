---
layout: default
title: "HaloPSA + Servosity in Hermes (Nous Research)"
description: "Install MSP Skills (HaloPSA tickets + Servosity backups) in Hermes, Nous Research's autonomous agent. Two paths: native Skill install or MCP server. Free, open source."
permalink: /integrations/hermes/
faqs:
  - q: "Does Hermes support HaloPSA?"
    a: "Yes. Hermes is Nous Research's autonomous agent. It speaks MCP natively and reads Claude-Code-compatible SKILL.md skills, so it runs MSP Skills' HaloPSA connector two ways - as a Skill install or as an MCP server."
  - q: "How do I install MSP Skills in Hermes?"
    a: "Either run 'hermes skills install Servosity/msp-skills/skills/halopsa --force' to install the Skill, or register the halopsa-mcp binary as an MCP server with 'hermes mcp add'. Both use the same HaloPSA / Servosity credentials."
  - q: "Is the Hermes install fully tested?"
    a: "The SKILL.md frontmatter and install sections ship today, and the MCP path is the reliable route. End-to-end Skill install from a monorepo subdirectory path is still being dogfooded upstream - if the Skill path fails, use the MCP path."
---

# HaloPSA and Servosity in Hermes

[Hermes](https://hermes-agent.nousresearch.com) is Nous Research's autonomous research agent. It's one of two skill-native agents (besides Claude) that read MSP Skills' `SKILL.md` directly **and** speak MCP - so you have two install paths.

## What you need

- **Hermes** installed (see [Nous Research docs](https://hermes-agent.nousresearch.com/docs))
- A **HaloPSA tenant + OAuth credentials** **or** a **Servosity MSP partner API token**
- A terminal once for install

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

## Path A - install as a Skill (uses the cli-printing-press template)

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/halopsa --force
hermes skills install Servosity/msp-skills/skills/servosity --force
```

Or inside a Hermes chat session:

```
/skills install Servosity/msp-skills/skills/halopsa --force
```

## Path B - register as an MCP server (most reliable)

Hermes speaks MCP natively, so it can use the binaries directly:

```bash
hermes mcp add halopsa -- halopsa-mcp
hermes mcp add servosity -- servosity-mcp
```

Set the same env vars the CLI needs (`HALOPSA_TENANT` / `HALOPSA_CLIENT_ID` / `HALOPSA_CLIENT_SECRET`, `SERVOSITY_MSP_TOKEN`).

## Step 3 - Ask a real question

- *"Use halopsa: triage what needs attention across all clients today."*
- *"Use servosity: show me stale backups across all clients this week."*

## Honest status

The cli-printing-press frontmatter + README install sections ship today, and the **MCP path (B) is the reliable route**. End-to-end Skill install from a non-canonical monorepo subdirectory (`Servosity/msp-skills/...`) has not been fully verified upstream - if Path A doesn't resolve, use Path B.

## What's next

- **Try a real workflow** at a free [Build Session](https://compoundingteams.com/build-sessions).
- **Full per-tool wire-up:** [Which AI agent?](/which-agent/)

[← Back to main install](/#install-in-60-seconds)
