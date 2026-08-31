---
layout: default
title: "Trust Center - Is it safe to give an AI agent access to my PSA? | MSP Skills"
description: "How MSP Skills connectors keep your client data safe: read/write/destructive command tiers, server-side API scoping enforced by your own tenant, open source with no telemetry and no servers of ours, and gitleaks + DCO + claims gates in CI. For the security-native MSP owner."
permalink: /governance/
faqs:
  - q: "Is it safe to give an AI agent access to my PSA?"
    a: "Yes, when access is scoped. Every command in an MSP Skills connector is tiered read, write, or destructive. The recommended agent policy lets reads run autonomously, requires a preview (--dry-run) for writes, and requires a human for anything destructive. The strongest control is server-side: you issue the API key in your own tenant and scope it there, so even a misbehaving agent can only do what your tenant permits."
  - q: "Does any data leave my network?"
    a: "Only if you route it out yourself. The CLI and MCP server are binaries on your own machine, any local data copy sits under your user account, and there are no servers of ours in the path - nothing phones home and there is no telemetry. Three paths can move data off the machine and each is one you switch on: --deliver webhook:<url> posts a command's output to a URL you name; a feedback endpoint you set in <PREFIX>_FEEDBACK_ENDPOINT, together with <PREFIX>_FEEDBACK_AUTO_SEND=true or --send, posts feedback you typed to that URL; and --transport http, on the connectors whose MCP server has it, opens a local listener you decide whether to expose. Credentials come from your own environment, never bundled into the repo or transmitted to us."
  - q: "How do I limit what the agent can do?"
    a: "Two layers. First, the connector's governance.md tags every command read, write, or destructive, and the recommended agent rule previews writes with --dry-run and requires your approval before any mutation. Second and strongest: scope the API key in your own vendor tenant so the connector can only reach what you granted. Server-side scoping your tenant enforces is the control that does not depend on the agent behaving."
  - q: "Could a connector update ship malicious code without me noticing?"
    a: "The whole repo is open source and inspectable, there is no telemetry and no auto-update phoning home, and every change runs through CI gates: gitleaks secret scanning, DCO sign-off, and a claims gate that proves the docs only describe commands the real binary has. Hand-fixes to generated code are recorded in a checked-in ledger so a regeneration can't silently drop a security fix."
---

<span class="eyebrow">TRUST CENTER · FOR THE SECURITY-NATIVE MSP</span>

# Is it safe to give an AI agent access to my PSA?

Yes - when access is scoped, and these connectors are built to scope it. Three things make that true: every command is tiered **read / write / destructive** so an agent only does what its tier allows; the strongest control is **server-side API scoping** your own tenant enforces; and the whole supply chain is **open source with no telemetry and no servers of ours**, gated in CI. Here is each layer in plain terms.

## The command tiers: what an agent may do on its own

Every connector ships a `governance.md` that tags every command into one of three tiers. The recommended agent policy maps each tier to how much autonomy it gets:

- **Read** - listing tickets, pulling a client card, running analytics. Changes nothing. Safe to run autonomously; this is where the day-to-day value lives.
- **Write** - creating a ticket, updating a record, posting a note. The recommended rule is to **preview with `--dry-run` and require your approval** before the agent commits any mutation.
- **Destructive** - deleting records, bulk operations that can't be undone. **Human-in-the-loop:** the agent surfaces what it would do and waits for a person.

This is a recommended policy your agent reads, not a hard sandbox - which is exactly why the next layer matters more.

## The strongest control: scope the key in your own tenant

The tiers above guide the agent. **Server-side API scoping does not depend on the agent behaving at all.** You issue the API key inside your own PSA, RMM, or backup tenant, and you scope it there - read-only, a single module, one client, whatever your vendor supports. The connector can never exceed what your tenant granted the key, no matter what an agent asks it to do.

This is the control a security-native buyer should lean on hardest: enforcement lives on **your** side of the wire, in a system you already trust to gate your techs.

## The supply-chain answer, head-on

A connector is code you run on your machine, so the fair question is: can I trust it?

- **Open source, inspectable.** Every line is on GitHub under Apache-2.0. Read it, fork it, pin a commit.
- **No telemetry. No servers of ours.** Nothing phones home. There is no hosted service in the path - out of the box the binaries talk only to your vendor's API with your credentials. The paths that can send data anywhere else are ones you switch on and point at a URL of your own: `--deliver webhook:<url>`, a feedback endpoint you set, and the `--transport http` MCP listener on the connectors that have it.
- **CI gates on every change.** `gitleaks` scans for leaked secrets, **DCO sign-off** attributes every commit, and a **claims gate** proves the docs only describe commands the real binary actually has - so the install instructions can't tell you to run something that doesn't exist.
- **Hand-fixes survive regeneration.** Connectors are generated, then carry hand-fixes for live-API quirks. Those fixes are recorded in a **checked-in ledger** that CI verifies, so a regeneration can't silently drop a security or correctness fix. [How fixes outlive a reprint →](/reprint-survival/)

## What an agent sees - and doesn't

When your AI answers a question, it sees the **result of a query**, not a raw dump of your database. A "stale backups across all clients" question returns the short list, not every backup record. Your credentials are read from your environment or your agent's config; they are never written into the repo or sent anywhere by us.

## Start read-only

The safe first move is a **read-tier triage against one client**: nothing changes, you see real output, and you decide from there. Then widen scope - and scope the API key in your tenant before you ever let an agent write.

Ready to look closer? [Browse the connectors →](/skills/) · [Why this layer at all →](/why/) · [See what's been verified live →](/verified/)
