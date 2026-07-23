# Security model - secrets via OpenCLI into the Keychain, never into context

## The one structural rule

`opencli browser <s> eval "<js>"` returns its result to **stdout**, which the Bash tool
hands back to the model. Therefore **the model must never be the process that reads a
secret's stdout.** Every secret flows through a shipped helper that consumes the value
*inside the script* and prints **only** a redacted receipt - `len`, `sha256[:8]`, `last4`.
The model authors the *selector* and the *destination* (keychain service/account); it never
sees the *value*. That process boundary is the whole security model.

`eval` has **no `--output`/file-sink flag** (verified), so this captured-inside-the-helper
pattern is the only way - and it works: see `scripts/grab_secret.sh --selfcheck` (asserts the
value never appears in stdout).

## Three lanes, in decision order

1. **Lane A - OAuth / CLI-owned loopback (preferred).** The consuming CLI's own
   `auth …-login` runs a `127.0.0.1` callback, catches the `code`, exchanges and stores the
   token itself. The token never renders in the DOM or in any stdout the model sees. Driver:
   `scripts/oauth_login.sh` backgrounds the (blocking) login, surfaces only the non-secret
   authorization URL; you drive the consent click (`ALLOW=authorize guard_click.sh`), then
   `--finish` confirms success from the CLI's own words. **After consent, do NOT inspect the
   page** - the `?code=` in the callback URL belongs to the CLI, not to you.
2. **Lane B - displayed value (`scripts/grab_secret.sh`).** A key is rendered on a settings
   page and there is no CLI auth subcommand. One shell process reads exactly one DOM node
   (selector base64-injected; **fails loudly on 0 or >1 matches**), pipes straight to
   `security add-generic-password -w`, prints only the receipt. `set +x` so a trace can't leak.
3. **Lane C - user paste (fallback).** One-time-shown / reveal-only / clipboard-only secrets,
   or any case where the Lane-B selector is unreliable. Print the bare-`-w` line for the user
   to run in their own terminal (cmux `!` prefix): `! security add-generic-password -U -a <acct>
   -s <SVC> -w` - a hidden prompt; the value never enters argv or context. Your only
   confirmation is the Lane-5 verification call.

**Escalate down on any doubt.** A truncated Lane-B token is worse than asking (Lane C).

## Residual surfaces (designed around - verified this session)

- **OpenCLI network cache.** `opencli browser … network` persists captured request/response
  bodies to `~/.opencli/cache/browser-network/<session>.json` (`chmod 600`). A token in an
  XHR body could land there. **Mitigations:** never run `network` / `console` / `get url`
  around a token exchange; never read that cache dir; use a per-target session name (not a
  long-lived shared one) so the cache is isolated; Lane A keeps the token off that wire
  entirely. If a run touched it, scrub `~/.opencli/cache/browser-network/<session>.json`.
- **`security … -w "$VALUE"` argv** is briefly visible in the *local* process table - on the
  user's own machine, not in model context. Acceptable. (Upgrade to a stdin/expect feed only
  if a shared host ever matters.)
- **Screenshots.** Never screenshot, `extract`, or `state` a page rendering a secret (a
  revealed key field, a token response). Proof is the redacted receipt, not a pixel.

## Verify by use, never by printing

A credential is unverified until a real, read-only authed call returns real data
(`scripts/verify_use.sh` - refuses secret-shaped fields, asserts a non-secret datum). The
log records `verified via <call> → <handle/email/id>`, never the token. To confirm two
captures match, compare `sha256[:8]`, never values.
