---
name: connect-tool
description: >
  Set up and PROVE authentication for any CLI, MCP server, or Skill by driving your
  ALREADY-OPEN, logged-in Chrome via the OpenCLI browser bridge (opencli browser bind):
  your real session, supervised live, never a fresh or headless Chromium. Reconciles to a
  desired auth state, so it works even when a tool is already connected: first-time setup,
  token refresh, broadening scopes, key rotation, and repair. Runs on macOS and Windows,
  storing every secret in the macOS Keychain or Windows Credential Manager without the
  complete value entering the model context, wires the consumer, and does not stop until a
  real authenticated call returns live data. Idempotent and self-learning.
  Use when the user says connect a vendor, set up auth, get me an API key or token, add or
  broaden a scope, refresh a token, log me into a CLI, wire up credentials, or do the auth
  setup. Drives your real Chrome through OpenCLI bind only. It does NOT and MUST NOT use any
  other browser-driving tool or skill, because a spawned browser does not carry your login.
allowed-tools: "Read, Write, Bash, PowerShell, AskUserQuestion"
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Servosity"
metadata:
  markdown_only: true
---

# connect-tool - browser-driven auth lifecycle manager

Drives your real, logged-in Chrome (via OpenCLI) to set up / refresh / broaden / repair
auth for any CLI, MCP server, or Skill; stores secrets in the OS credential store
**without the complete value ever entering this context**; and does not stop until a real
authenticated call returns live data. **Runs on macOS and Windows.**

## 0. Running the helpers (read before your first command)

Every helper is a Python script next to this file, run the same way on both platforms:

```
uv run <this-skill-dir>/scripts/<helper>.py [args]
```

`<this-skill-dir>` is the directory containing this SKILL.md, which you already know
because you just read it. Use that path directly. Do NOT search the filesystem for it and
do NOT hardcode `~/.claude/skills/connect-tool`: a plugin install lands somewhere else,
and any shell-search snippet breaks under the PowerShell tool on Windows.

`uv` supplies its own Python, so nothing else needs installing to run these. If `uv` is
absent but Python 3.12+ is present, `python3 <script>` (`python` on Windows) also works.

There is no bash in these instructions on purpose. On native Windows without Git for
Windows, Claude Code has no Bash tool at all and uses PowerShell; a `uv run` line is
identical in both shells.

**Requirements: macOS or Windows, Google Chrome, Node.js 20+ and npm, OpenCLI plus its
Chrome extension, and `uv` (or Python 3.12+).** Check every one in a single command:
`uv run <this-skill-dir>/scripts/preflight.py --deps`. Setup detail is in `README.md`.

## 1. HARD GUARDRAIL - browser pinning (read first)

**DO NOT use any other browser tool: no Playwright, no Puppeteer, no headless Chromium, no
cookie-import helper, no other browser-driving skill.** They open a *separate* browser with
**no login**, which is the "a browser I did not ask for just started" bug. This skill uses
**only** `opencli browser <session> bind` against the tab you already have focused. If you
catch yourself about to launch a browser any other way, **STOP, wrong tool.** The only
browser entry point in this skill is `scripts/preflight.py` (which binds your real Chrome).

**A new blank Chrome window is NOT this bug.** OpenCLI's extension manages its own container
windows, so `bind` may surface a fresh `about:blank` window instead of the tab you focused
(upstream `jackwener/opencli#2202`; issue #266 was filed over exactly this). That window is in
the user's **own Chrome profile**, so it carries their logins - **say that out loud** instead
of letting them conclude the run went wrong, and follow the recovery order in
`references/browser-and-keychain.md`. What IS the bug is a *separate browser*, which is what
the paragraph above forbids.

If the separate **`opencli-browser`** skill is also installed, **this skill's rules win**
for anything in a connect-tool run. That skill teaches free use of `eval`, `network`,
`console`, and `extract`, which is exactly what section 2 forbids around a secret.

## 2. Core principles

- **The complete secret never enters context.** Never run `opencli browser … eval` on a
  secret node yourself, never read the clipboard, never screenshot/`extract`/`state`/
  `network` a page showing a secret. All secret capture goes through `grab_secret.py` /
  `oauth_login.py`, which print only a redacted receipt (`len`/`sha8`/`last4`). See
  `references/security-model.md`, including what this does NOT claim.
- **Idempotent + lifecycle.** Reconcile to the desired state; re-runs do the minimal delta.
- **Hold the irreversible.** Agree scope up front; never click post/publish/save/pay/delete.
  Surface it instead. Drive consent clicks through `guard_click.py`.
- **Never "done" without a live receipt.** A real authenticated read must return real data.
- **Log structured events as you go** (`audit_log.py`), with no secret values, ever.

## 3. The reconcile loop (desired vs current, to one operation)

`uv run scripts/reconcile.py --target T --scopes a,b` reads per-target state and emits:

| Result | Operation | Browser? |
|---|---|---|
| no prior state | `setup` | yes |
| token valid, granted scopes cover desired | `noop` | no |
| expired/expiring + refresh available | `refresh` | no |
| desired scopes not all granted | `broaden` (incremental consent) | yes |
| expired, no refresh | `reauth` | yes |
| error_count_7d >= 3 | `repair` (surface, suggest reset) | no |

"Already set up" is never a dead end. Read-only overview: `uv run scripts/state.py current`.

## 4. Phase workflow

Run a target through these phases; loop 3-5 until the verify receipt passes.

0. **Agree** (AskUserQuestion): target, exact scopes, destination (Keychain default),
   keychain account/service names, and the hold-list. Open a run dir
   under the platform runs dir (`uv run scripts/ctplatform.py` documents it); write `STATE.md`.
1. **Read learnings + load state:** `uv run scripts/learning.py guidance --target T`,
   then `reconcile.py` for the operation. If `noop`, report and stop.
2. **Pre-flight + bind:** `uv run scripts/preflight.py <target-slug>` (binds your focused
   Chrome). If OpenCLI is missing or disconnected it refuses and prints the setup steps;
   walk the user through `references/opencli-bootstrap.md` rather than installing anything
   unasked. Never fall through to another browser tool.
3. **Drive** the operation. Navigate with `opencli browser <slug> open|state|find|click|fill|
   upload|wait` (crib: `references/browser-and-keychain.md`). If a recipe exists use its nav;
   else discover live from `state`/`find`/`extract`. Route every click that could be
   irreversible through `uv run scripts/guard_click.py <slug> "<selector>"`.
4. **Capture the secret out-of-context** by lane (decision order in `security-model.md`):
   - **Lane A (preferred), OAuth:** three calls, because the consent click happens
     between them:
     1. `RUN_DIR=$RUN uv run scripts/oauth_login.py --start --session <slug> -- <cli auth login ...>`
        spawns a background broker, navigates your bound tab to the consent page, and
        RETURNS `AUTH_NAVIGATED`. It never prints the URL, which carries an OAuth `state`.
     2. Drive the consent click: `ALLOW=authorize uv run scripts/guard_click.py <slug> "<selector>"`.
     3. `RUN_DIR=$RUN uv run scripts/oauth_login.py --finish` reports `OAUTH_OK` or fails.
        The token is never read, and the CLI's raw output is never written to disk.
   - **Lane B, displayed key:** `uv run scripts/grab_secret.py --session <slug>
     --selector '<css>' --service <SVC> --account <acct>`.
   - **Lane C, user paste:** print the one-line store command for the user to run in their
     OWN terminal (in Claude Code, prefix it with `!`), with a hidden prompt so the value
     never enters argv or this context. macOS:
     `security add-generic-password -U -a <acct> -s <SVC> -w`. Windows: have them paste it
     into `uv run scripts/credstore.py` interactively, or use Lane B.
   Then **wire** the consumer: `uv run scripts/mint_wrapper.py <name> <ENV_VAR> <acct> <SVC>
   <absolute-binary>` for a CLI, or `claude mcp add ... --` pointing at that launcher (never
   put the value in the MCP config file).
5. **Verify (the receipt):** `uv run scripts/verify_use.py <non-secret-field-path> --
   <read-only authed cmd>` (a strict dotted path such as `.data.id`, not a jq filter).
   Must return live data. On 401/403, re-drive / re-scope (back to 3). **Never report
   working without this.**
6. **Persist + report:** append target state (`uv run scripts/state.py append '<json>'`,
   refs/scopes/expiry only, no values), write/refresh the learned recipe on first success,
   `learning.py record` any lesson, finalize `REPORT.md`, then
   `opencli browser <slug> unbind` (detach, do not close the tab).

## 5. Learning (compounds every future setup)

- **Start:** inject prior lessons with `learning.py guidance`.
- **End / on any correction:** `uv run scripts/learning.py record --lesson "…" --kind correction
  --tags <provider>,<scheme>` (add `--global` for universal lessons like "read the DOM, never
  pbpaste"). Per-target state lives in `targets.jsonl`; lessons live in the shared feedback
  substrate at `~/.claude/learning/feedback.jsonl`.
- **Periodically:** `uv run scripts/patterns.py` mines the audit logs across runs for
  recurring failures and proposes new global lessons (human-ratified with `--record`).

## 6. Safety / refusal

Stop and ask when: an action is off the agreed scope; a HOLD fires (irreversible verb); the
selector for a secret is ambiguous (Lane B fails on any match count other than exactly 1, so
escalate to Lane C); or three real auth attempts fail (surface the audit trail, do not claim
done).

## References (load as needed; this file stays the contract)

- `references/security-model.md` - the three secret lanes, residual surfaces, mitigations.
- `references/browser-and-keychain.md` - OpenCLI command crib + Keychain no-echo conventions.
- `references/state-recipes-audit.md` - targets.jsonl + recipe + reconcile + audit-event schema.
- `references/opencli-bootstrap.md` - install + configure OpenCLI when missing/disconnected.
- `references/windows.md` - what differs on Windows, and what is unverified there.
