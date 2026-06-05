---
name: msp-skills-concierge
description: "Use when the user has msp-skills installed and wants help choosing or installing connectors - it reads the live catalog, learns their PSA/RMM/backup/security/billing stack, recommends the connectors that fit, and installs only the ones they approve. Trigger phrases: `recommend which connectors I should install`, `which msp-skills connector for my stack`, `set up the right connectors for me`, `concierge`, `msp-skills concierge`, `what connectors should I install`, `pick connectors for my MSP`, `using everything you know about me, recommend connectors`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Servosity"
allowed-tools: "Read Bash WebFetch"
metadata:
  markdown_only: true
---

# msp-skills Concierge

The concierge recommends which msp-skills connectors fit the user's stack and installs only the ones they approve. It has no binary of its own; it reads the live catalog and drives each connector's own installer.

## The magic prompt

This is the moment this skill exists for. When the user says something like:

> You have msp-skills installed. Using everything you know about me and how I work, recommend which connectors I should install - and install the ones I approve.

run the workflow below.

## Workflow

### 1. Fetch the LIVE catalog

Always read the catalog from the source of truth on `main`, never a copy bundled in this skill (a bundled copy rots):

```bash
curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/catalog.json
```

If that fetch fails (offline, network error, non-200), fall back to the local repo copy at `catalog.json` in the repository root if one is present. If neither the live fetch nor a local copy is available, say so plainly and STOP - do not guess at a connector list from memory.

The catalog's `skills[]` array gives you, per connector: `name`, `system`, `status`, `vendor`, `category`, `tagline` (when present), `description`, the install one-liner (`install_skill_one_liner`), and the per-agent wire-up doc path (`install_mcp_doc`). A connector may carry `markdown_only: true` (a meta-skill like this one) - never recommend a markdown-only entry as a connector.

### 2. Learn the user's stack

If the conversation or context already names the user's tools (their PSA, RMM, backup, security, or billing vendors), use what you already know - do not re-ask. Only if the stack is genuinely unknown, ask AT MOST 2-3 short questions, for example:

- Which PSA do you run? (HaloPSA, ConnectWise, Autotask, ...)
- Which RMM, and which backup/BCDR tool?
- Any security or billing tools you'd want your AI to reach (e.g. M365 security, Pax8)?

Keep it to one short message. Do not interrogate.

### 3. Match stack to catalog and present a shortlist

Match the named vendors and categories against the catalog. Rank by relevance (exact vendor match first, then same-category, then adjacent). Present a shortlist as a table:

| Connector | What it answers | Verification |
| --- | --- | --- |
| (name) | (the connector's tagline - the question it answers) | (honest badge, see below) |

State the verification badge HONESTLY from the catalog data:

- If the connector's `live_verified.status` is `verified`, write `Live-verified` plus the date.
- Otherwise write `Awaiting live verification - passes every mechanical gate`.

Never inflate a badge. "Awaiting live verification" is the truthful state for most connectors and it is not a weakness - it is the signal MSP feedback closes.

When the catalog or the corpus at `https://raw.githubusercontent.com/Servosity/msp-skills/main/docs/llms-full.txt` carries an instead-of / just-say example for a connector, include ONE per recommendation so the user sees the concrete payoff (for example: instead of exporting three reports and pivoting them in Excel, just say "Which clients had backup-failure tickets last quarter?").

### 4. Install only what the user approves

Wait for the user to approve specific connectors. Install ONLY those. For each approved connector:

- On macOS / Linux, run its own installer:
  ```bash
  bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/<slug>/install.sh)
  ```
- On Windows, run the PowerShell installer instead:
  ```powershell
  iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/<slug>/install.ps1 | iex
  ```

After each install, print the per-agent wire-up pointer so the user can connect the connector to their agent: the connector's `mcp-install.md` at `https://github.com/Servosity/msp-skills/blob/main/skills/<slug>/mcp-install.md`.

### 5. Receipts, not claims

After each install, prove the binary is present by running its version command and REPORTING the output verbatim:

```bash
<slug>-cli --version
```

If the user has already configured the connector's credentials per its README, also run its health check and report the output:

```bash
<slug>-cli doctor
```

Never claim a connector is "working" or "shipped" on the strength of an install alone. The binary being present is a receipt that it installed; closure is the USER actually running it against their tenant. Report what the commands returned and let the user judge.

### 6. Refuse-by-design

- Never install a connector the user did not explicitly approve.
- Never write, store, echo, or transmit credentials anywhere - the user enters their own credentials following each connector's README.
- If the catalog fetch fails and there is no local copy, say so and stop. Do not improvise a connector list.
- Never recommend or install a `markdown_only` entry as a connector.
