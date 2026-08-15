---
layout: default
title: "grab-cookie - re-auth the MSP tools that never issue an API key | MSP Skills"
description: "For the tools in your stack that authenticate with a browser session and nothing else: paste one DevTools Copy as cURL and grab-cookie extracts the httpOnly cookie or opaque bearer token, stores it in Windows Credential Manager or the macOS Keychain, rebuilds the config that consumes it, and proves it with a real authenticated call. A daily doctor warns you before the session expires instead of after. Free, open source, contributed by BomberJacket Networks."
permalink: /skills/grab-cookie/
skill_name: "grab-cookie"
faqs:
  - q: "How do I connect a tool that has no API key and no OAuth?"
    a: "Borrow the session your browser already holds. Log into the site in Chrome, open DevTools, right-click any API request and choose Copy as cURL (bash), and paste it into a capture file. grab-cookie parses the request, pulls out the httpOnly cookie or bearer token per a small JSON profile you write once for that site, stores it in the Windows Credential Manager or the macOS Keychain, regenerates whatever config file consumes it, and then makes a real authenticated call to prove the credential works before it reports success."
  - q: "Why can't the cookie be captured automatically?"
    a: "An httpOnly cookie is invisible to page JavaScript by design, so no bookmarklet or extension script can read it, and that restriction is doing its job. A headless browser does not carry your login either, and automating the login means storing the actual password and breaking on the first MFA prompt. These sessions were built for a human in a browser, so the paste stays manual. Everything after the paste - extraction, storage, wiring, verification, expiry warning - is automated."
  - q: "Should I use grab-cookie or connect-tool?"
    a: "connect-tool, whenever your vendor offers an API key or OAuth. It handles scope changes and rotation properly and deliberately refuses cookie import, which is the right call for its threat model. grab-cookie exists for the vendors that leave you no such path. A session credential is a bearer of your entire login rather than a scoped key: it carries whatever that account can do, it cannot be narrowed, and you revoke it by logging the session out. Use it only where there is no alternative."
  - q: "Where does grab-cookie put the credential?"
    a: "In your operating system's credential store - the Windows Credential Manager, or the macOS Keychain. On Windows it is written through the platform credential API rather than a command line, so the secret does not appear in process-creation logs. The complete value is never printed, never logged, and never returned into the agent's context: a receipt carries a length, a short hash prefix, and the last four characters, and for a short secret both the hash prefix and the last four are withheld so the receipt cannot be brute-forced back into the value. Config files that consume the credential are written 0600 on macOS and Linux and regenerated from the store on demand, so they are a cache rather than a second copy of record."
  - q: "Does grab-cookie work on macOS?"
    a: "Partly, and the part that is proven is the storage layer. The skill is verified on Windows against the Windows Credential Manager. On macOS, running the self-check with --live creates, reads back, and deletes a real Keychain entry, and it passes - so the credential store works there. What has not been exercised on macOS is a full seed against a real site. Note the two self-check modes: plain --selfcheck runs offline checks only and touches no credentials, while --selfcheck --live is the one that proves the platform path. The live-verified badge is separate again, and only a real MSP's own report earns it."
---

# grab-cookie

**Re-auth the tools that never issue an API key, in one paste.**

Two or three tools in most MSP stacks authenticate with a browser session and nothing else: an httpOnly cookie, or an opaque token parked in localStorage. About once a month the session expires, nothing announces it, and a scheduled job quietly starts returning empty results. grab-cookie turns that recovery into one DevTools paste and one command, ending in a real authenticated call that either passes or fails loudly.

It is a free, open source [Claude Code Skill](/install-skill/), contributed by BomberJacket Networks. Markdown plus four stdlib Python files: no compiled binary, no MCP server, no third-party packages.

## Use connect-tool first, and this only when you cannot

If your vendor issues an API key or supports OAuth, [connect-tool](/skills/connect-tool/) is the answer and this skill is the wrong tool. connect-tool handles scope changes and rotation properly, and it refuses cookie import on purpose.

A session credential makes the opposite tradeoff, deliberately and in writing:

| | An API key via connect-tool | A session credential via grab-cookie |
|---|---|---|
| **What it can do** | Whatever scope you granted it. | Whatever the logged-in account can do. It cannot be narrowed. |
| **How you revoke it** | Delete the key in the vendor portal. | Log the session out. |
| **How long it lasts** | Until you rotate it. | Weeks, typically, then it dies without warning. |
| **When to reach for it** | Always, when the vendor offers it. | Only when the vendor offers nothing else. |

The pain is real, and refusing to cover it does not stop people doing it worse by hand - in a text file, next to the code, with no verification and no expiry warning. This skill covers the case honestly instead.

## What the paste actually buys you

The capture is irreducible. Everything around it is not, and that is where the afternoon goes:

- **Finding the secret inside the request.** A `Copy as cURL` is the real request the browser sent, httpOnly cookie included. A per-site JSON profile says which part of it matters.
- **Getting it into the credential store** rather than a text file, without the value passing through the agent's context.
- **Rebuilding the config that consumes it** - in the right file, in the right format, with or without the `Bearer` prefix, decided once in the profile instead of remembered monthly.
- **Proving it works** with a real authenticated call before you walk away. A stored credential that does not actually work is worse than none, because it fails later and somewhere else.
- **Warning you five days out** instead of a week after the numbers started looking wrong.

Adding a site is adding a profile, not changing code. Two annotated examples ship in `profiles/`.

## Being honest about the edges

- **Windows is verified. macOS is proven at the storage layer only.** `--selfcheck --live` round-trips a real Keychain entry and passes; a full seed against a real site has not been run on a Mac. Plain `--selfcheck` is offline-only - `--live` is the one that proves the platform path.
- **A capture file holds a live credential** until you delete it - the whole DevTools request, so every cookie for that origin and every auth header, not just the one value. A shipped `.gitignore` covers `captures/`, `*.curl.txt`, and `state.json`; delete the capture once the seed verifies.
- **This skill does not drive a browser.** It reads a cURL command you paste. Nothing automates your login, and nothing stores your password.
- **Apache-2.0 and inspectable**, four stdlib Python files with a `--selfcheck --live` you can run rather than trust.

## Install

Install the Skill, then write a profile for your site. Full steps are in the [grab-cookie README on GitHub](https://github.com/servosity/msp-skills/tree/main/skills/grab-cookie).

Then tell your agent what broke:

> the reporting tool stopped pulling data again - re-auth it

It walks you through the DevTools paste, seeds the credential, rebuilds the config, and finishes with the authenticated call that proves it.

[grab-cookie on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/grab-cookie) &nbsp; [Browse all skills →](/skills/)
