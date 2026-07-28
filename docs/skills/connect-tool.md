---
layout: default
title: "connect-tool - get any vendor's API key connected, and prove it worked | MSP Skills"
description: "The last-mile credential step for MSP AI setups. connect-tool drives the Chrome you are already logged into, puts a displayed API key or pasted secret straight into Keychain or Windows Credential Manager without the value passing through the agent, and for a new or changed connection will not report connected until a real authenticated read returns your live data. Free, open source, Windows and macOS."
permalink: /skills/connect-tool/
skill_name: "connect-tool"
faqs:
  - q: "How do I actually get my vendor's API key into an MCP server or CLI?"
    a: "connect-tool does it. It drives the Chrome tab you are already logged into, reads the displayed key (or takes a hidden paste), and writes it straight into the macOS Keychain or Windows Credential Manager. The agent never sees the full value, and the key never lands in a config file or MCP JSON. It then runs a real authenticated read and only reports success once your live data comes back."
  - q: "Why not just ask Claude to connect the tool for me?"
    a: "Ask an agent to connect a vendor and it usually spawns a fresh browser with none of your logins, hits a login wall, or reports connected when nothing authenticated, with your API key sitting in a config file. connect-tool uses your already-signed-in browser, keeps the complete secret out of the agent's context, and will not claim success on a new connection until a real authenticated call returns your data."
  - q: "Where does connect-tool store my credential?"
    a: "In your operating system's credential store: the macOS Keychain, or Windows Credential Manager via a direct CredWriteW call so the secret is never a command-line argument. A displayed or pasted key goes there, not into a config file. OAuth logins are handled by the consuming CLI's own login, which stores its own token."
  - q: "Is connect-tool safe to run against my real vendor portals?"
    a: "It is Apache-2.0 and inspectable, and every helper ships a --selfcheck you can run to verify the security properties rather than trust them. The complete secret never enters the agent's context, irreversible clicks are routed through a hold you approve per step, and it stores nothing in this repo at runtime. It does depend on OpenCLI and its Chrome extension, which sit in your credential path and are worth pinning and reviewing."
  - q: "Does connect-tool work on Windows?"
    a: "It runs on Windows and macOS from one codebase, with the credential going to Windows Credential Manager. On macOS every helper's self-check passes, including a live credential-store round-trip; the Windows-specific paths are new and still being verified on real hardware. The skill's references/windows.md lists exactly what is proven and what is not."
---

# connect-tool

**Get any vendor's API key connected to your AI, and prove it actually worked.**

Every connector you install has the same last mile: get a credential out of a vendor portal, put it somewhere safe, wire it into the tool, and confirm the tool actually works. That last mile is where most setups die, usually with an agent cheerfully reporting "connected" when nothing was ever authenticated.

connect-tool does that last mile for the CLIs, MCP servers, and Skills that are **not** already built-in Claude connectors. It drives the Chrome you are already logged into, so you watch it happen. A displayed key or a pasted secret goes to your operating system's credential store without the complete value ever passing through the agent's context. And for a new or changed connection, it will not report success until a real authenticated read returns real data from the vendor.

It is a free, open source [Claude Code Skill](/install-skill/) (and works with any agent that reads Skills). There is no MCP server of its own to install, because it has no API of its own: it connects the tools that do.

## Why this over just asking your agent to "connect X for me"

Ask an agent to connect a vendor with no defined auth workflow, and the failure modes are familiar: it reports "connected" when nothing authenticated, or your API key ends up in a config file. connect-tool is that defined workflow. For a new or changed connection it will not report success until a real authenticated call returns your data, it stores a displayed or pasted key in the OS credential store, and it uses the browser you are already signed into.

| | Just ask your agent to "connect X" | connect-tool |
|---|---|---|
| **Proof it worked** | It says "connected." | For a new or changed connection it will not report success until a real authenticated read returns your live data. |
| **Where your key goes** | Through the agent's context, often into a config file or MCP JSON. | A displayed key or pasted secret goes straight into Keychain / Credential Manager; the agent sees only a length and a hash prefix (plus the last four for longer secrets). |
| **Whose session** | A spawned browser opens with none of your logins and hits a wall. | It drives the Chrome tab you are already signed into. |
| **Running it again** | Re-auths blindly or makes duplicates. | It reads what it already did and picks the smallest next step: nothing, refresh, broaden scopes, re-auth, or flag a repair. |
| **What it will click** | Anything. | The workflow routes irreversible clicks (save / pay / delete / revoke) through a hold you approve per step. A discipline, not a sandbox. |
| **Next time** | Nothing carries over. | Lessons the agent records ("this vendor hides the key under Configuration > Integrations") come back on later runs. |

**Where it does not earn its keep:** when a tool is already a built-in first-party Claude connector, you just click connect and OAuth handles the rest. connect-tool is for the vendor tools that are not built in, whether they use a displayed API key, a pasted secret, or their own OAuth CLI login (HaloPSA, NinjaOne, Jamf, Servosity, and the like).

## Why credential-store-not-config-file matters at MSP scale

An MCP server config with a plaintext API key is a liability, and you multiply it by every client you manage. connect-tool puts a displayed or pasted key into the OS credential store and wires the consuming tool through a launcher that reads it at start-up, so the value does not sit in a config file waiting for an auditor to find it. On Windows the credential is written through a direct `CredWriteW` call, so the secret is never even a command-line argument.

## Being honest about the edges

This audience checks claims rather than trusting them, which is the whole point, so:

- It needs OpenCLI and its Chrome extension: a third-party npm package and a Chrome Web Store extension sit in your credential path. Pin the versions you review.
- The browser bridge has a documented, recoverable failure where its background worker goes to sleep (you reload the extension's card in `chrome://extensions/`). Test the reliability in your own environment before you lean on it.
- Apache-2.0 and inspectable, with a `--selfcheck` on every helper, so you can verify these properties instead of taking them on faith. Real assurance is the whole helper chain plus OpenCLI and the consuming CLI, not one file.
- Windows support is new and still being verified on real Windows hardware. On macOS, every helper's `--selfcheck` passes, including a live credential-store round-trip.

## Install

Install the Skill, then the two runtime pieces (Node 20+, `uv`, OpenCLI), then add the OpenCLI Chrome extension in one click. On Windows there is a no-Git bootstrap. Full steps, including the dependency check you can ask your agent to run, are in the [connect-tool README on GitHub](https://github.com/servosity/msp-skills/tree/main/skills/connect-tool).

Then log in to the vendor portal in Chrome, leave that tab focused, and tell your agent what you want:

> connect the halopsa CLI

It agrees the scope with you up front, drives your own browser while you watch, and ends with a receipt: a real authenticated call and the live data that came back.

[connect-tool on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connect-tool) &nbsp; [Browse all connectors →](/skills/)
