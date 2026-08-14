---
name: grab-cookie
description: >
  Capture, store, wire, and verify a browser-session credential for any site that
  has no API key and no OAuth path: an httpOnly session cookie, an opaque
  localStorage bearer token, or a custom auth header. The human step is one
  DevTools "Copy as cURL" paste, and everything around it is automated: parse the
  request, extract the secret per a per-site JSON profile, store it in the Windows
  Credential Manager or the macOS Keychain without the value entering the agent's
  context, regenerate whatever config file consumes it, and prove the result with
  a real authenticated call. A daily doctor re-probes every stored credential and
  warns before a known expiry instead of after.
  Use when the user says a tool stopped working after about a month, my session
  expired, re-auth this site, the cookie died again, refresh my token, this vendor
  has no API key, or set up auth for a site that only works when logged in.
  This Skill does NOT drive a browser. It reads a cURL command the user pastes.
  For vendors that DO expose OAuth or an API key, use connect-tool instead.
allowed-tools: "Read, Write, Bash, PowerShell, AskUserQuestion"
author: "Mike Bramm"
license: "Apache-2.0"
vendor: "BomberJacket Networks"
metadata:
  markdown_only: true
---

# grab-cookie - session credentials for sites with no API path

Some vendors never issue an API key. Their web app authenticates with an httpOnly
session cookie or an opaque token in localStorage, and the only way to reach the
API your agent needs is to borrow the session the browser already has.

That capture is irreducibly human: an httpOnly cookie is invisible to page
JavaScript by design, and a headless browser does not carry your login. What is
NOT irreducible is everything around it. This Skill automates the rest, so a
thirty-day re-auth costs one paste instead of an afternoon of remembering where
the token was supposed to go.

## When to use this Skill, and when not to

Use it when the vendor has no API-key or OAuth path for the surface you need, and
the working credential exists only inside a logged-in browser session.

Do not use it when the vendor does issue an API key or supports OAuth. The
`connect-tool` Skill in this repository handles those properly, including scope
changes and rotation. `connect-tool` deliberately refuses cookie import and never
reads `document.cookie` or `localStorage`, which is the correct choice for its
threat model. This Skill covers the case that choice leaves open, and it makes
the opposite tradeoff explicitly.

## 0. Running the helpers

Every helper is a stdlib Python script next to this file. Python 3.12 or newer,
no third-party packages, no compiled binary, no MCP server:

```bash
python "<this-skill-dir>/scripts/credgrab.py" <command> [args]
```

`<this-skill-dir>` is the directory containing this SKILL.md, which you already
know because you just read it. Use that path directly rather than searching the
filesystem, and do not hardcode an install location: a plugin install lands
somewhere other than `~/.claude/skills/grab-cookie`.

On Windows use `python`; on macOS use `python3`.

## 1. Pick or write a profile

A profile is one JSON file describing a single site: where the secret lives in
the request, where the consuming config file is, and how to prove the credential
works. Adding a site means adding a profile, not changing code.

Two annotated examples ship in `profiles/`:

- `example-cookie-site.json` for an httpOnly session cookie
- `example-bearer-token.json` for an opaque bearer token plus a second header

Copy one, rename it, and edit the four blocks: `extract`, `wire`, `verify`, and
the optional `ttl_days`.

## 2. Capture

Tell the user, in these words or close to them:

> Log into the site in Chrome. Press F12, open the Network tab, click any request
> to the site's API, right-click it, and choose Copy as cURL (bash). Paste it into
> `captures/<profile>.curl.txt` and save.

Copy as cURL is the right capture surface because DevTools shows the real request
the browser sent, including the httpOnly cookie that no script can read. The
parser accepts both the bash and cmd variants, since which one a user gets depends
on their platform. Choose **Copy as cURL (bash)** when it is offered: "Copy as
PowerShell" is a different menu item that emits `Invoke-WebRequest` rather than a
curl command, carries no `-H` flags, and parses to nothing.

The capture file is disposable. Delete it after seeding.

## 3. Seed

```bash
# macOS: use python3
python "<this-skill-dir>/scripts/credgrab.py" seed --profile <name> --curl captures/<name>.curl.txt
```

This parses the request, extracts each configured value, writes it to the OS
credential store, regenerates the consumer file from what was stored, and then
runs the profile's verify command. It prints a receipt (length, a short hash
prefix, last four characters) and never the secret.

If verification fails, the seed is reported as failed. A stored credential that
does not actually work is worse than no credential, because it fails later and
somewhere else.

## 4. Re-wire without re-capturing

```bash
python "<this-skill-dir>/scripts/credgrab.py" wire --profile <name>
```

The credential store is the source of truth, so a consumer config file can be
deleted, corrupted, or excluded from a backup and rebuilt with no browser step.
This is the command to reach for when a tool suddenly cannot authenticate but the
session itself has not expired.

## 5. Verify and monitor

```bash
python "<this-skill-dir>/scripts/credgrab.py" verify --profile <name>
python "<this-skill-dir>/scripts/credgrab.py" doctor --all
```

`doctor --all` is what you schedule. It probes every stored credential and reports
those that are dead or close to a known expiry, which converts a silent thirty-day
breakage into a warning that arrives before the tool stops working.

Wire it to whatever notifier you already use. A daily run is usually enough for a
thirty-day session.

## 6. Self-check

```bash
python "<this-skill-dir>/scripts/credgrab.py" --selfcheck
```

Round-trips a synthetic value through the real credential store on this machine
and deletes it. Run this first on a new machine, before assuming a failure is the
vendor's fault.

## Security posture, stated plainly

- The secret is written to the OS credential store through the platform API. On
  Windows that is `CredWriteW` with the value passed as a memory buffer rather
  than on a command line, so it does not land in process-creation logs.
- The complete value is never printed, never logged, and never returned into the
  agent's context. Receipts carry length, a hash prefix, and the last four
  characters, which is enough to tell two credentials apart and not enough to use
  one.
- Capture files hold a real credential until deleted. They belong in
  `.gitignore`, and the Skill treats them as disposable.
- Consumer config files are written with mode 0600 and are regenerated on demand,
  so they can be treated as a cache rather than as a secret store.
- This Skill adds no browser automation and no network listener. It reads a file
  the user pasted.

## What this does not do

It does not refresh a session automatically. No refresh token exists for these
sites, which is the entire reason the tool exists. It shortens and schedules the
human step; it does not remove it. Any tool claiming otherwise for an httpOnly
session cookie is either driving a browser or storing your password.
