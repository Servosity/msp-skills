> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/domain-cert-tracker/

# The Domain and Certificate Tracker MCP Server - free, local, built for MSPs

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of
> Servosity Inc. Apache-2.0 licensed.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Domain and Certificate Tracker. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Domain and Certificate Tracker to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "what expires, lost its lock or changed across my clients' domains this month?" and get one answer per client: registration expiry and transfer lock from RDAP, every unexpired certificate logged in Certificate Transparency, the certificate each host really serves, and DNS drift. It needs no account or credential: every source is public, and the inventory and history stay on your machine.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/domain-cert-tracker/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/domain-cert-tracker)

## Instead of clicking through Domain and Certificate Tracker, just ask

**Instead of** Keeping a spreadsheet of client domains and certificate expiry dates and checking it by hand before each renewal
**just ask:** *"What expires in the next 30 days across all my clients, and did any domain lose its transfer lock?"*
Your agent runs: `domain-cert-tracker-cli report --days 30 --agent`

**Instead of** Finding out an ACME renewal never reached the server when the client's site starts showing a certificate warning
**just ask:** *"Which hosts are still serving an old certificate after a newer one was issued?"*
Your agent runs: `domain-cert-tracker-cli renewals --agent`

**Instead of** Searching crt.sh one domain at a time to spot a certificate nobody on the team ordered
**just ask:** *"Did any client get a certificate from a CA their CAA record does not allow, or from a new issuer?"*
Your agent runs: `domain-cert-tracker-cli issuers audit --days 7 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/domain-cert-tracker/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What expires within 30 days, lost its lock or changed, per client? | `domain-cert-tracker-cli report --days 30 --agent` |
| Which hosts still serve an old certificate after a newer one was logged? | `domain-cert-tracker-cli renewals --agent` |
| Which new certificates came from an issuer the domain's CAA does not permit, or a first-ever issuer? | `domain-cert-tracker-cli issuers audit --days 7 --agent` |
| Which hostnames have certificates in CT but are not in the client's inventory? | `domain-cert-tracker-cli hosts discover --agent` |
| What changed since the agent last looked, once per event, for ticketing? | `domain-cert-tracker-cli events list --since 0 --agent` |
| What certificate does this host serve right now, and when does it expire? | `domain-cert-tracker-cli tls probe www.example.com --agent` |
| What are a domain's registrar, expiry date and lock statuses? | `domain-cert-tracker-cli domain example.com --json` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/domain-cert-tracker/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/domain-cert-tracker/guide.md).

## What makes this one different

This connector keeps a per-client inventory and a local history: sweep checks RDAP, Certificate Transparency, live TLS and DNS for every client domain and host, records each result, and raises typed change events, so report, renewals and issuers audit answer from the store without a network call. Single-lookup tools such as openrdap, crt-mcp or ssl-watch answer one domain or one host at a time and keep no client grouping or history.

There is no single vendor here: registries publish RDAP, crt.sh indexes the public Certificate Transparency logs, and your hosts serve TLS. Registrar and CA portals each show their own slice for one account; this joins all of them per client and works with no account at any of them.

## The pain this closes

- Certificate lifetimes are shrinking under CA/Browser Forum ballot SC-081v3: 200 days from March 2026, 100 days from March 2027 and 47 days from March 2029. Renewal becomes a monthly automated job, and each automation can fail without telling anyone.
- Domains and certificates for dozens of clients sit across many registrars and CAs, so the expiry list usually lives in a spreadsheet or a calendar that someone has to keep current by hand.
- A renewed certificate that never reached the web server looks fine in the CA's portal until the old one expires and the client calls.
- MSPs on r/msp still ask for a free SSL expiration tracker that makes sense, because single-lookup tools answer one domain at a time and keep no per-client history.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domain-cert-tracker/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domain-cert-tracker/install.ps1 | iex
```

There is nothing to authenticate: every source is public. Verify the install with `domain-cert-tracker-cli doctor`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | report, renewals, issuers audit, hosts discover, events list, clients list, domain, tls probe, dns check, certificates | Allow |
| Write (routine, local store only) | sweep | Allow; it records only this tool's own sweep history |
| Write (inventory, local store only) | clients set (previews unless --yes or confirm: true), clients import (command line only) | Allow after confirm; adds to the inventory every sweep checks |
| Destructive (local store only) | clients remove (previews unless --yes or confirm: true; a whole client only with --whole-client), local-data purge (dry run unless --yes) | Human-in-the-loop only |

It reads public RDAP, Certificate Transparency, DNS and TLS and never changes anything at a registrar, CA, DNS provider or client server. Its only writes are to its own local store: the client inventory (clients set, clients import, clients remove) and sweep history. Let agents run reads and sweeps; changing or removing inventory needs an explicit confirm, and purging the local store is for a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/domain-cert-tracker/governance.md).

## Frequently asked questions

### Is there an MCP server for Domain and Certificate Tracker?

Yes - this one. A free, open source MCP server and Claude Code Skill for Domain and Certificate Tracker, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Domain and Certificate Tracker MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DOMAIN_CERT_TRACKER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DOMAIN_CERT_TRACKER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the domain-cert-tracker MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. There are no credentials to enter.

### Do I need an account or API key?

No. RDAP, crt.sh and DNS are public, and TLS probes connect to your clients' own public hosts. There is nothing to sign up for and no credential to store.

### Is my client data safe?

The client inventory and sweep history stay in a local SQLite store under your user account. Lookups send only the domain or host being checked to the public service that answers for it. HTTP lookups and TLS probes connect only to public addresses on port 443 unless you allow another port on the command line (the one exception is a server you name yourself in DOMAIN_CERT_TRACKER_BASE_URL); DNS checks use the resolver your operating system is configured with.

### Will it hit rate limits?

sweep sends each RDAP lookup straight to the registry that answers for the domain (through the IANA bootstrap file), runs at most 4 checks in parallel, and downloads at most 50 new certificates per sweep from crt.sh to read their full name lists. A crt.sh answer cut off at its 10,000-row limit is a failed check. A failed check is listed as unverified and never replaces the last good result.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Monitoring connectors

Run more than one Monitoring tool, or comparing options? These connectors work the same way: [Better Stack](https://msp-skills.compoundingteams.com/skills/betterstack.md) · [Windows Server](https://msp-skills.compoundingteams.com/skills/windows-server.md)

## Status

Beta. Validated against the Domain and Certificate Tracker API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
