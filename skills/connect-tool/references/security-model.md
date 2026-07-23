# Security model - secrets into the OS credential store, never into context

## The one structural rule

`opencli browser <s> eval "<js>"` returns its result to **stdout**, which the shell tool
hands back to the model. Therefore **the model must never be the process that reads a
secret's stdout.** Every secret flows through a shipped helper that consumes the value
*inside the process* and prints **only** a redacted receipt: `len`, `sha256[:8]`, `last4`.
The model authors the *selector* and the *destination* (service/account); it never sees the
*value*. That process boundary is the whole security model.

`eval` has **no `--output`/file-sink flag** (verified), so this captured-inside-the-helper
pattern is the only way, and it works: see `uv run scripts/grab_secret.py --selfcheck`,
which asserts the value never appears in stdout and does a real round-trip through the
credential store.

Structural supports for that boundary, all enforced in code:

- no `shell=True` anywhere, so a selector or URL is never parsed by `cmd.exe` or bash
- fixed error strings; a captured value is never interpolated into an exception message
- captured output is never logged, and never returned up the stack

## What this does NOT claim

Being precise here matters more than sounding strong.

- **"The complete secret never enters the agent's context."** That is the claim. The
  redacted receipt (`len`, `sha256[:8]`, `last4`) is deliberate: `last4` is how an operator
  matches a stored key against the masked value a vendor portal displays, and `sha256[:8]`
  lets two captures be compared without either being shown. Below 12 characters the last
  four would be a meaningful fraction of the whole, so `last4` is withheld entirely. If you
  want no receipt at all, use Lane C and type the secret yourself.
- **The consumer is a separate trust boundary.** Once the launcher hands the credential to
  the consuming CLI as an environment variable, the value lives in that process. If that CLI
  prints it on `--debug`, dumps it in a crash, or ships telemetry, that is outside what
  connect-tool can control.
- **Python strings cannot be reliably zeroized.** The Windows credential buffer is wiped
  best-effort after the write; the Python string it came from is not. A heap dump of a
  running helper could contain a secret. This was equally true of the previous shell
  implementation, and of essentially every credential tool not written in a language with
  secure memory primitives.
- **Deleting a file is not shredding it**, especially on an SSD. Lane A therefore never
  writes the consuming CLI's raw output to disk in the first place, rather than writing and
  deleting it.

## Three lanes, in decision order

1. **Lane A - OAuth / CLI-owned loopback (preferred).** The consuming CLI's own
   `auth ... login` runs a `127.0.0.1` callback, catches the `code`, and exchanges and
   stores the token itself. The token never renders in the DOM or in any stdout the model
   sees. Driver: `scripts/oauth_login.py` spawns the (blocking) login detached, consumes its
   output stream continuously **without keeping it**, and **navigates your bound tab to the
   authorization URL rather than printing it** (an authorize URL carries an unguessable
   `state`, plus tenant and client identifiers, none of which belong in model context). You
   drive the consent click with `ALLOW=authorize`, then call `--finish`; success is read
   from the CLI's own words, with negations ("not authenticated") rejected first. `--start`
   and `--finish` are separate invocations with a detached broker in between, precisely so
   the consent click can happen while the login is still waiting on its callback. **After consent, do NOT inspect the
   page:** the `?code=` in the callback URL belongs to the CLI, not to you.
2. **Lane B - displayed value (`scripts/grab_secret.py`).** A key is rendered on a settings
   page and there is no CLI auth subcommand. One process reads exactly one DOM node (the
   selector is base64-injected so no quoting can break out of the page script, and `--attr`
   is enum-checked before it reaches that script; **fails loudly on 0 or more than 1
   match**), writes it straight to the credential store, and prints only the receipt. The
   value is not stripped, because leading or trailing whitespace can be significant.
3. **Lane C - user paste (fallback).** One-time-shown, reveal-only, or clipboard-only
   secrets, or any case where the Lane-B selector is unreliable. The user runs this in their
   OWN terminal (in Claude Code, prefix it with `!`), at a hidden prompt, so the value never
   enters argv or context:

   ```
   uv run <skill-dir>/scripts/credstore.py --store --service <SVC> --account <acct>
   ```

   On macOS `security add-generic-password -U -a <acct> -s <SVC> -w` is equivalent. Your
   only confirmation is the Lane-5 verification call.

**Escalate down on any doubt.** A truncated Lane-B token is worse than asking (Lane C).

## Where the secret is stored

| Platform | Store | Secret in a command line? |
|---|---|---|
| macOS | Keychain, via `security add-generic-password` | **Yes, briefly.** See the residual below |
| Windows | Credential Manager, via `CredWriteW` through `ctypes` | **No.** Passed as a memory buffer |

The Windows path is the stronger of the two, and deliberately so. `ctypes` is used instead
of PowerShell `Add-Type` P/Invoke for two reasons: Constrained Language Mode, which
AppLocker and WDAC commonly induce on a managed endpoint, blocks Win32 P/Invoke from
PowerShell entirely; and calling the API directly from Python means the secret is never an
argument to any process, so it can never appear in a Sysmon Event 1 or a 4688 audit record.

Windows credentials are written with `CRED_PERSIST_LOCAL_MACHINE`, not `ENTERPRISE`:
`ENTERPRISE` roams with a roaming profile, which would carry the secret to every machine
the user signs into. They are visible and revocable in the Credential Manager UI under
`Servosity/connect-tool/<service>/<account>`. Never put tenant data in a target name.
Credential Manager caps a generic blob at 2560 bytes; anything larger fails loudly rather
than being truncated.

After an administrative password reset, a stored credential may become unreadable. Treat a
read failure as revocation and re-authenticate; do not assume either platform's store
survives it.

## Residual surfaces (designed around)

- **OpenCLI network cache.** `opencli browser ... network` persists captured
  request/response bodies to `~/.opencli/cache/browser-network/<session>.json`. A token in
  an XHR body could land there. **Mitigations:** never run `network` / `console` / `get url`
  around a token exchange; never read that cache dir; use a per-target session name (not a
  long-lived shared one) so the cache is isolated; Lane A keeps the token off that wire
  entirely. If a run touched it, scrub that file.
- **`security ... -w <value>` argv on macOS** is briefly visible in the *local* process
  table, on the user's own machine, not in model context. Accepted. On an endpoint with
  process-creation auditing this would be recorded, which is exactly why the Windows path
  does not use argv at all.
- **Screenshots.** Never screenshot, `extract`, or `state` a page rendering a secret (a
  revealed key field, a token response). Proof is the redacted receipt, not a pixel. HOLD
  screenshots go to a private run directory (mode 0700 on POSIX, an explicit DACL on
  Windows), never to a world-readable temp dir.
- **The `opencli-browser` skill**, if installed, teaches free use of `eval`, `network`, and
  `console`. Inside a connect-tool run, this skill's rules win.
- **Batch shims on Windows.** Launching a `.cmd` or `.bat` goes through `cmd.exe`, which
  re-parses arguments even when Python is told not to use a shell: an `&` inside an OAuth
  URL could split the command. So when npm has installed `opencli.cmd`, the helpers resolve
  its JavaScript entry point and run it through `node.exe` directly, which takes a real
  argv. `mint_wrapper.py` likewise refuses to target a `.cmd`/`.bat` binary.

## Verify by use, never by printing

A credential is unverified until a real, read-only authed call returns real data
(`scripts/verify_use.py`). That helper takes a **strict dotted path** (`.data.id`), not
filter code: there is no way to select the whole response, construct a secret-shaped key
dynamically, or return a container. Secret-shaped path segments are refused
case-insensitively, the asserted value must be a bounded, printable scalar, and the VALUE
is checked as well as the field name, so `{"value": "sk_live_..."}` is refused rather than
printed. The same value-shape check guards the audit log and the state file, so a token
written into an innocently-named field is refused rather than persisted. The log
records `verified via <call> -> <handle/email/id>`, never the token. To confirm two captures
match, compare `sha256[:8]`, never values.
