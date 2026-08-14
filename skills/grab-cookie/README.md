# grab-cookie - session credentials for sites that never issue an API key

> Unofficial. Community-built Claude Code Skill, contributed by BomberJacket Networks, Inc. Not affiliated with, endorsed by, or sponsored by any vendor whose site it may be used with. Vendor names, where they appear, are trademarks of their respective owners and are used descriptively only.

Some tools in an MSP stack never issue an API key. The web app logs you in with a
session cookie the browser marks httpOnly, or an opaque token parked in
localStorage, and that session is the only way to reach the API. Roughly once a
month it expires, something breaks quietly, and you rediscover where the token
was supposed to go.

This Claude Code Skill automates everything around the one step that cannot be
automated. You paste a "Copy as cURL" from DevTools. It does the rest: extract the
secret, store it in the Windows Credential Manager or the macOS Keychain, rebuild
whatever config file consumes it, and prove the result with a real authenticated
call. A daily doctor re-probes what it stored and warns you before an expiry
rather than after.

## Why the human step cannot be removed

An httpOnly cookie is invisible to page JavaScript by design, so no bookmarklet
can read it. A headless browser does not carry your login, so no automated
capture reaches it either. And these vendors issue no refresh token, because they
never intended a machine to hold the session at all.

So the capture stays manual. What does not have to stay manual is the extraction,
the storage, the wiring, the verification, and the expiry warning, which is where
the actual time goes.

## How it relates to connect-tool

The `connect-tool` Skill in this repository handles vendors that expose OAuth or
an API key, and it deliberately refuses cookie import. That is the right call for
its threat model, and this Skill does not change it.

grab-cookie covers the case that choice leaves open, and makes the opposite
tradeoff on purpose and in writing. If your vendor has an API key, use
`connect-tool`. This is for the ones that do not.

It reuses `connect-tool`'s credential-store backend unmodified except for the
namespace, so both Skills write to the same OS credential store without
colliding. See NOTICE.

## What ships

Markdown plus four stdlib Python files. No compiled binary, no MCP server, no
third-party packages, no browser automation, and no network listener. Python 3.12
or newer.

A site is described by one JSON profile covering where the secret is in the
request, which config file consumes it, and how to prove it works. Adding a site
is adding a profile. Two annotated examples ship in `profiles/`.

## Platform support

Verified on Windows, against the Windows Credential Manager.

The macOS Keychain path is `connect-tool`'s own backend, carried over with only a
namespace change, so it is expected to work. It has not been exercised on macOS by
the contributor, who has no Mac to test on. Treated as untested until someone
runs `--selfcheck` there and says otherwise. Reports welcome.

## Security posture

The secret is written through the platform credential API rather than a command
line, so on Windows it does not appear in process-creation logs. The complete
value is never printed, never logged, and never returned into the agent's
context; receipts carry a length, a hash prefix, and the last four characters.
Capture files hold a live credential until deleted and belong in `.gitignore`.
Consumer config files are written 0600 and regenerated from the credential store
on demand, so they are a cache rather than a second copy of record.

## Quick start

```bash
python "<this-skill-dir>/scripts/credgrab.py" --selfcheck
cp profiles/example-cookie-site.json profiles/mysite.json
# edit profiles/mysite.json, then paste a Copy as cURL into captures/mysite.curl.txt
python "<this-skill-dir>/scripts/credgrab.py" seed --profile mysite --curl captures/mysite.curl.txt
python "<this-skill-dir>/scripts/credgrab.py" doctor --all
```

Schedule the last line daily and the thirty-day surprise becomes a warning.

## License

Apache-2.0. See NOTICE for attribution of the vendored credential-store backend.
