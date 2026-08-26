---
layout: default
title: "How to contribute to MSP Skills - start with an it-works receipt | MSP Skills"
description: "The four ways to contribute to MSP Skills, ordered by how much they help: send an it-works receipt from your real tenant, report a real-tenant bug, open a small fix or docs pull request, or add a whole connector. The full contributor guide lives in CONTRIBUTING.md at the repo root."
permalink: /contributing/
---

# How to contribute to MSP Skills

The single most valuable contribution is not code: it is one sentence from an MSP who ran a connector against a real tenant and watched it work. Most connectors here were built without access to the vendor's system, so a live receipt is the only thing that can flip a **Live-verified** badge. Reporting one takes about 60 seconds and needs no terminal.

**[Read the full contributor guide (CONTRIBUTING.md) &rarr;](https://github.com/servosity/msp-skills/blob/main/CONTRIBUTING.md)**

## The four rungs

1. **Send an "it works" receipt.** You ran a connector against your tenant and it did the job. [Fill in the form](https://github.com/servosity/msp-skills/issues/new?template=it-works.yml), or email hello@servosity.com. Your name goes on [the receipts wall](/verified/).
2. **Report a real-tenant bug.** Something misbehaved against a live tenant. [Fill in the bug-report form](https://github.com/servosity/msp-skills/issues/new?template=bug-report.yml) with the command you ran and the exact error.
3. **Open a small fix or docs pull request.** A corrected example, a clearer sentence, a live-API quirk. Hand-fixes to generated connector code get recorded in `handfixes.json` so a regeneration cannot silently drop them ([why](/reprint-survival/)).
4. **Add a connector or a Skill.** Either shape is first-class: a Go CLI plus MCP server, or a markdown-only Skill with no binary at all.

## Where things live

- **The contributor guide**, with every rung in detail plus sign-off, the security gate, the non-affiliation banner, and the skill layout: [CONTRIBUTING.md](https://github.com/servosity/msp-skills/blob/main/CONTRIBUTING.md).
- **Which connectors carry a live receipt** and which are awaiting their first: [the receipts wall](/verified/).
- **Requesting a system we do not cover yet**: [the 90-second walkthrough](/requesting-a-skill/).
- **Building one with us, live**: [Build Sessions](/build-sessions/).
- **A security problem**: email security@servosity.com rather than filing publicly.
