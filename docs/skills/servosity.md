---
layout: default
title: "Servosity + AI - for Claude, ChatGPT, Codex, Cursor, and any MCP agent"
description: "Free Claude Code skill and MCP server for Servosity backups. Fleet-wide attention, stale-backup detection, overnight drift, per-client situational awareness, cross-engine analytics. Local fleet mirror - Friday-email-ready in seconds."
permalink: /skills/servosity/
skill_name: "Servosity MCP"
---

# Servosity + AI in 60 seconds

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of Servosity Inc. Apache-2.0 licensed.

Add **fleet-wide backup triage, stale-backup-set detection, overnight drift, per-client situational awareness, and cross-engine analytics** to the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. A **local fleet mirror** means your AI can answer cross-client questions the partner portal can't show on one screen - **Friday-email-ready in seconds**. Built for MSP partners. No code required.

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/servosity){: .btn}

## What it does

| Question your MSP keeps asking | Command |
| --- | --- |
| What needs my attention across every client this morning? | `servosity-cli attention` |
| Which backups went stale across all clients? | `servosity-cli stale-backups --days 7` |
| What changed across my fleet overnight? | `servosity-cli drift --from yesterday --to now` |
| What's the whole story for this client, on one screen? | `servosity-cli company show <id>` |
| Search across companies, backups, and issues at once | `servosity-cli find "image manager"` |
| Triage what's worth keeping in the alert queue | `servosity-cli triage` |
| Clear known-safe alert noise so real failures show | `servosity-cli clear` |
| List DR restores in flight | `servosity-cli restore-queue list` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md).

## What makes this different

Most AI integrations for backup and DR vendors proxy each question into a live API call against the partner portal. That's fine for one client. It fails when you're asking *"which of my 47 clients have a backup set that hasn't succeeded in 7 days, sliced by engine?"* - the partner portal scatters that across per-client pages, three engine views, and an alert queue full of noise.

This skill syncs Servosity into a **local fleet mirror** with snapshot history and FTS5 search. Cross-engine, cross-client questions become one local query: instant, offline, and the AI sees the answer, not a context window full of paginated JSON.

## The pain this closes

Backup and DR is where "silent failure" hurts most:

- **Silent backup failures discovered too late.** A backup that quietly stopped succeeding is invisible until a client needs a restore - the worst possible moment to find out.
- **No fleet-wide view.** Each client's backup state lives in its own portal view; there's no single screen that says "across my whole book, here's what's stale, failing, or in-flight right now."
- **Alert-queue noise buries the real failure.** Dozens of repeat and known-safe issues pile up per client; the one that matters hides in the pile.
- **Per-client questions mean portal archaeology.** Answering "is this client OK?" means clicking through metadata, three backup engines, contracts, and issues by hand.

`attention` is one screen across every client. `stale-backups` is the Friday-email list, ready. `drift` opens Monday with situational awareness instead of a blank slate. `triage` / `clear` / `stale-issues` batch-suppress known-safe noise. `company show` / `find` answer per-client and cross-fleet questions in one command.

## Install

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Claude Cowork** | [Step-by-step →](/integrations/cowork/) |
| **GitHub Copilot** (VS Code) | [Step-by-step →](/integrations/github-copilot/) |
| **Microsoft 365 Copilot / Copilot Studio** | [Step-by-step →](/integrations/microsoft-365-copilot/) (remote) |
| **Google Gemini** | [Step-by-step →](/integrations/gemini/) |
| **Hermes**, **OpenClaw** | [Hermes →](/integrations/hermes/) · [OpenClaw →](/integrations/openclaw/) |
| Cursor, Windsurf, Cline, Continue, Zed | [Which agent? →](/which-agent/) |

**Quickest path** (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

After install: get your partner API token from the Servosity partner portal, then:

```bash
SERVOSITY_MSP_TOKEN=<token> servosity-cli doctor
```

`doctor` confirms the token works and the local mirror is reachable before you run anything that touches client data.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `attention`, `drift`, `stale-backups`, `backup-facts`, `find`, `company show`, `restore-queue list` | Allow |
| Write (routine) | `triage`, `clear`, `stale-issues`, notes/comments | Allow with `--confirm`; log the plan first |
| Credential / security | credentials rotate/delete, MFA, agent-install-token, encryption-key update | Human-in-the-loop only |
| Destructive | `companies delete`, `backups delete`, restic-prune, `users delete` | Human-in-the-loop only |
| Admin (hidden) | `admin ...` | Operator-only, not for agents |

Token is scoped to **your reseller account only** (no cross-reseller access). Full matrix in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/governance.md).

## Status

Already used inside Servosity's own backup/DR operations; published here for MSP partners. The public surface is in beta and being validated with MSPs in live **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
