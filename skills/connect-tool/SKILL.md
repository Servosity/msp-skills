---
name: connect-tool
description: >
  Set up and PROVE authentication for any CLI, MCP server, or Skill by driving your
  ALREADY-OPEN, logged-in Chrome via the OpenCLI browser bridge (opencli browser bind):
  your real session, supervised live, never a fresh or headless Chromium. Reconciles to a
  desired auth state, so it works even when a tool is already connected: first-time setup,
  token refresh, broadening scopes, key rotation, and repair. Stores every secret in the
  macOS Keychain without the value entering the model context, wires the consumer, and does
  not stop until a real authenticated call returns live data. Idempotent and self-learning.
  Use when the user says connect a vendor, set up auth, get me an API key or token, add or
  broaden a scope, refresh a token, log me into a CLI, wire up credentials, or do the auth
  setup. Drives your real Chrome through OpenCLI bind only. It does NOT and MUST NOT use any
  other browser-driving tool or skill, because a spawned browser does not carry your login.
allowed-tools: "Read, Write, Bash, AskUserQuestion"
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Servosity"
metadata:
  markdown_only: true
---

# connect-tool - browser-driven auth lifecycle manager

Drives your real, logged-in Chrome (via OpenCLI) to set up / refresh / broaden / repair
auth for any CLI, MCP server, or Skill; stores secrets in the macOS Keychain **without the
value ever entering this context**; and does not stop until a real authenticated call
returns live data.

`SKILL_DIR` is the directory this SKILL.md lives in. Resolve it once at the start of a
run and use it for every helper call; do NOT hardcode a path, because it differs by
install method:

```bash
SKILL_DIR="${CLAUDE_PLUGIN_ROOT:-$HOME/.claude/skills/connect-tool}"   # plugin install, else manual install
[ -f "$SKILL_DIR/SKILL.md" ] || SKILL_DIR=$(dirname "$(find "$HOME/.claude" -name SKILL.md -path '*connect-tool*' 2>/dev/null | head -1)")
```

Run Python helpers with `uv run` (or `python3` 3.12+), shell helpers with `bash`.

**Requirements: macOS, Google Chrome, Node.js + npm, OpenCLI + its Chrome extension, `uv`
or Python 3.12+, and `jq`.** Check them all in one shot with
`bash "$SKILL_DIR/scripts/preflight.sh" --deps`. Setup detail is in `README.md`.

## 1. HARD GUARDRAIL - browser pinning (read first)

**DO NOT use any other browser tool: no Playwright, no Puppeteer, no headless Chromium, no
cookie-import helper, no other browser-driving skill.** They open a *separate* browser with
**no login**, which is the "a browser I did not ask for just started" bug. This skill uses
**only** `opencli browser <session> bind` against the tab you already have focused. If you
catch yourself about to launch a browser any other way, **STOP, wrong tool.** The only
browser entry point in this skill is `scripts/preflight.sh` (which binds your real Chrome).

## 2. Core principles

- **Secrets never enter context.** Never run `opencli browser … eval` on a secret node
  yourself, never `pbpaste`, never screenshot/`extract`/`state`/`network` a page showing a
  secret. All secret capture goes through `grab_secret.sh` / `oauth_login.sh`, which print
  only a redacted receipt (`len`/`sha8`/`last4`). See `references/security-model.md`.
- **Idempotent + lifecycle.** Reconcile to the desired state; re-runs do the minimal delta.
- **Hold the irreversible.** Agree scope up front; never click post/publish/save/pay/delete.
  Surface it instead. Drive consent clicks through `guard_click.sh`.
- **Never "done" without a live receipt.** A real authenticated read must return real data.
- **Log structured events as you go** (`audit_log.sh`), with no secret values, ever.

## 3. The reconcile loop (desired vs current, to one operation)

`uv run "$SKILL_DIR/scripts/reconcile.py" --target T --scopes a,b` reads per-target state and emits:

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
   `RUN="$HOME/.config/connect-tool/runs/<target>-$(date -u +%Y-%m-%d-%H%M)"`; write `STATE.md`.
1. **Read learnings + load state:** `uv run "$SKILL_DIR/scripts/learning.py" guidance --target T`,
   then `reconcile.py` for the operation. If `noop`, report and stop.
2. **Pre-flight + bind:** `bash "$SKILL_DIR/scripts/preflight.sh" <target-slug>` (binds your
   focused Chrome). If OpenCLI is missing or disconnected it refuses and prints the setup
   steps; walk the user through `references/opencli-bootstrap.md` rather than installing
   anything unasked. Never fall through to another browser tool.
3. **Drive** the operation. Navigate with `opencli browser <slug> open|state|find|click|fill|
   upload|wait` (crib: `references/browser-and-keychain.md`). If a recipe exists use its nav;
   else discover live from `state`/`find`/`extract`. Route every click that could be
   irreversible through `bash scripts/guard_click.sh <slug> "<selector>"`.
4. **Capture the secret out-of-context** by lane (decision order in `security-model.md`):
   - **Lane A (preferred), OAuth:** `RUN_DIR=$RUN bash scripts/oauth_login.sh --start -- <cli auth login …>`
     then drive consent with `ALLOW=authorize guard_click.sh`, then `oauth_login.sh --finish`.
   - **Lane B, displayed key:** `bash scripts/grab_secret.sh --session <slug> --selector '<css>'
     --service <SVC> --account <acct>`.
   - **Lane C, user paste:** print the bare `security add-generic-password -U -a <acct> -s <SVC> -w`
     for the user to run in their own terminal (in Claude Code, prefix it with `!`).
   Then **wire** the consumer: `bash scripts/mint_wrapper.sh …` for a CLI, or
   `claude mcp add … --` pointing at a keychain-backed wrapper command (never put the value
   in the MCP config file).
5. **Verify (the receipt):** `bash scripts/verify_use.sh <non-secret-jq-field> -- <read-only authed cmd>`.
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
