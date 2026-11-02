> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/grab-cookie/

# grab-cookie

**Re-auth the tools that never issue an API key, in one paste.**

Two or three tools in most MSP stacks authenticate with a browser session and nothing else: an httpOnly cookie, or an opaque token parked in localStorage. About once a month the session expires, nothing announces it, and a scheduled job quietly starts returning empty results. grab-cookie turns that recovery into one DevTools paste and one command, ending in a real authenticated call that either passes or fails loudly.

It is a free, open source [Claude Code Skill](https://msp-skills.compoundingteams.com/install-skill.md), contributed by BomberJacket Networks. Markdown plus four stdlib Python files: no compiled binary, no MCP server, no third-party packages.

## Use connect-tool first, and this only when you cannot

If your vendor issues an API key or supports OAuth, [connect-tool](https://msp-skills.compoundingteams.com/skills/connect-tool.md) is the answer and this skill is the wrong tool. connect-tool handles scope changes and rotation properly, and it refuses cookie import on purpose.

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

[grab-cookie on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/grab-cookie)   [Browse all skills →](https://msp-skills.compoundingteams.com/skills/)
