# grab-cookie - session credentials for sites that never issue an API key

> Unofficial. Community-built Claude Code Skill, contributed by BomberJacket Networks, Inc. Not affiliated with, endorsed by, or sponsored by any vendor whose site it may be used with. Vendor names, where they appear, are trademarks of their respective owners and are used descriptively only.

> **Use `connect-tool` instead whenever your vendor has an API key or OAuth.** A session
> credential is a bearer of your entire login, not a scoped key: it carries whatever that
> account can do, it cannot be narrowed, and revoking it means logging the session out.
> [connect-tool](../connect-tool/) covers every vendor that offers a real key and refuses
> cookie import on purpose. grab-cookie is for the vendors that leave you no such path, and
> it makes that tradeoff deliberately and in writing. Treat a capture file as live
> credential material: `.gitignore` it, and delete it once seeded.

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

It is forked from `connect-tool`'s credential-store backend rather than reused
unmodified: the namespace differs (`credgrab/...`, so the two Skills never
collide in the OS credential store), the live self-check is opt-in here, and
there are four further behavioural differences. NOTICE lists every one.

## What ships

Markdown plus four stdlib Python files. No compiled binary, no MCP server, no
third-party packages, no browser automation, and no network listener. Python 3.12
or newer.

A site is described by one JSON profile covering where the secret is in the
request, which config file consumes it, and how to prove it works. Adding a site
is adding a profile. Two annotated examples ship in `profiles/`.

## Platform support

Verified on Windows, against the Windows Credential Manager.

On macOS the credential-store round trip is verified: `--selfcheck --live`
creates, reads back, and deletes a real Keychain entry, and it passes
(`credstore.py selfcheck OK (backend=macos-keychain, round-trip + no leak)`).
What is still unexercised on macOS is a full seed against a real site, so treat
the platform as proven at the storage layer and unproven end to end. Reports
welcome.

Note that plain `--selfcheck` runs the OFFLINE checks only and never touches the
credential store - it is `--selfcheck --live` that proves the platform path. The
split is deliberate: a live check can raise a macOS Keychain access prompt, which
should not happen behind a command documented as touching no credentials.

## Security posture

The secret is written through the platform credential API rather than a command
line, so on Windows it does not appear in process-creation logs. The complete
value is never printed, never logged, and never returned into the agent's
context; receipts carry a length, a hash prefix, and the last four characters.
Capture files hold a live credential until deleted and belong in `.gitignore`.
Consumer config files are written 0600 on macOS and Linux and regenerated from
the credential store on demand, so they are a cache rather than a second copy of
record. On Windows the mode is best-effort: the file inherits the NTFS ACLs of
its parent directory, which for the default location under your user profile is
already user-only. `state.json`, which records a receipt per seeded profile, is
written through the same 0600 path.

## Quick start

```bash
python "<this-skill-dir>/scripts/credgrab.py" --selfcheck --live   # --live proves the credential store
cp profiles/example-cookie-site.json profiles/mysite.json
# edit profiles/mysite.json, then paste a Copy as cURL into captures/mysite.curl.txt
python "<this-skill-dir>/scripts/credgrab.py" seed --profile mysite --curl captures/mysite.curl.txt
python "<this-skill-dir>/scripts/credgrab.py" doctor --all
```

Schedule the last line daily and the thirty-day surprise becomes a warning.

## License

Apache-2.0. See NOTICE for attribution of the vendored credential-store backend.
