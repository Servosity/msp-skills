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
2. **Report a real-tenant bug.** Something misbehaved against a live tenant. [Fill in the bug-report form](https://github.com/servosity/msp-skills/issues/new?template=bug-report.yml) with the command you ran and what came back - error text if there was any, otherwise the wrong or empty result.
3. **Open a small fix or docs pull request.** A corrected example, a clearer sentence, a live-API quirk. Hand-fixes to generated connector code get recorded in `handfixes.json` so a regeneration cannot silently drop them ([why](/reprint-survival/)).
4. **Add a connector or a Skill.** Either shape is first-class: a Go CLI plus MCP server, or a markdown-only Skill with no binary at all.

## Where things live

- **The contributor guide**, with every rung in detail plus sign-off, the security gate, the non-affiliation banner, and the skill layout: [CONTRIBUTING.md](https://github.com/servosity/msp-skills/blob/main/CONTRIBUTING.md).
- **Which connectors carry a live receipt** and which are awaiting their first: [the receipts wall](/verified/).
- **Requesting a system we do not cover yet**: [the 90-second walkthrough](/requesting-a-skill/).
- **Building one with us, live**: [Build Sessions](/build-sessions/).
- **A security problem**: email security@servosity.com rather than filing publicly.

## Copyright and how you get credit

These are two different things, and the project keeps them separate on purpose.

**Copyright is uniform.** Wherever a copyright line appears in a connector - a
`.go` file under `skills/*/cli/`, that connector's `cli/NOTICE`, its `cli/LICENSE`
attribution line - it reads the same as the root `LICENSE`:

```
Copyright 2026 Servosity Inc. and msp-skills contributors
```

You are one of those contributors the moment your PR merges. Individual names do
not go in per-file headers: a header naming one person is wrong the first time
somebody else edits that file, and it tells anyone adopting the code the wrong
thing about who owns that subtree. `tools/maintainer/check_copyright.py` runs on
every pull request and enforces this, because the generator stamps whoever ran it
and a regeneration would otherwise undo it. Those three sets of files are its
whole scope: its rule is that any copyright line present in them must be the
canonical one, it does not require a file to carry one, and it does not scan
Markdown, Python or shell.

**Credit is personal**, and lives in three places:

| Where | What it looks like |
| --- | --- |
| `skills/<slug>/cli/NOTICE` | `The Avanan connector was contributed by Abhi Saini (@geekbrownbear).` |
| `SKILL.md` frontmatter | `author: "Abhi Saini"` |
| Git history + your DCO sign-off | the permanent, legal record |

`NOTICE` is the Apache-2.0 attribution channel (section 4(d)). A reprint
regenerates it from the press template, so every credited connector pins its
"contributed by" sentence in `skills/<slug>/handfixes.json` and
`check_handfixes.py` fails CI if a regeneration drops the line. That is what makes
"survives a regeneration" a checked promise rather than an intention.

A ledger assert counts its marker in the whole file, comments included, so an
assert on a code symbol that a doc comment also names can pass after the code is
gone. Add `"code_only": true` to that assert to count comment-stripped source
instead, or `"code_only": false` when the comment **is** what must survive. It
must be a JSON boolean, and it belongs only on a `contains` assert - a
`not_contains` is always checked whole-file, which is already its strongest
form. The full schema, and the advisory
`check_handfixes.py --lint-asserts` report, are in
[reprint survival](/reprint-survival/).

Where that credit reaches today: `cli/NOTICE` is in the repository and in every
source checkout. The release assets are the two binaries plus their `.sha256`
files, and the `.mcpb` bundle is `manifest.json` plus those binaries, so no
release asset ships `NOTICE` or `LICENSE` yet. Adding both alongside the binaries
is a planned release-pipeline change, not a fact about today's downloads.

Tell us in the PR how you want to be named - handle, real name, company, or not at
all - and that is what we will use.

### If you are adopting a skill rather than contributing one

Everything here is Apache-2.0, and that license is what governs your use. The
uniform copyright line means there is no per-file patchwork to audit and no
individual holding rights over one subdirectory, though it is a summary of
ownership rather than a substitute for the license text and the git history. No
contributor license agreement was signed. Contributors sign off each commit under
the [Developer Certificate of Origin](https://developercertificate.org), which
certifies they have the right to submit the work; Apache-2.0 is the license the
work is submitted under.
