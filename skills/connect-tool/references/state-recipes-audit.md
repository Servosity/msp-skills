# State, recipes, reconcile, and the audit-event schema

All runtime data lives **outside the repo**, under `~/.config/connect-tool/` on macOS and
`%LOCALAPPDATA%\Servosity\connect-tool\` on Windows (`$CONNECT_TOOL_HOME` overrides the
root, `$CONNECT_TOOL_STATE_DIR` just the state dir). It holds **no secret values**, only
references, scopes, expiry, app ids. Generic lessons live in the shared feedback substrate
(`~/.claude/learning/feedback.jsonl`), not here.

## Per-target state - `state/targets.jsonl` (append-only, newest-per-target wins)

Idempotency substrate. `scripts/state.py`:
```bash
uv run scripts/state.py append '{"target":"halopsa","status":"authenticated", ...}'
uv run scripts/state.py current [target]      # newest entry (one target) or full map
uv run scripts/state.py journal-append '{"target":"halopsa","step":"token_stored","status":"ok"}'
uv run scripts/state.py journal halopsa
```
Per-entry fields (no secret values - `state.py` refuses secret-shaped keys):
```
target, flow_type, status, app_id,
scopes_requested[], scopes_granted[], scopes_pending[],
auth_method, token_type,
access_token_keychain_ref, refresh_token_keychain_ref, client_secret_keychain_ref,
token_expiry_ts (ISO or null), refresh_capable (bool),
last_verified_ts, verification_result, account_id, account_email,
last_error, error_count_7d
```
`status` values: `app_not_created`, `app_ready`, `authenticated`, `token_expired`,
`needs_scope_increment`, `auth_error`.

`setup-journal.jsonl` records multi-step progress (`app_created` → `consent` → `token_stored`)
so a partial failure resumes from the dead step instead of restarting.

## Reconcile - desired vs current → the operation

`scripts/reconcile.py --target T --scopes a,b` (reads `targets.jsonl`, or pass `--current '<json>'`):

| Current | Operation | Browser |
|---|---|---|
| absent | `setup` | yes |
| `error_count_7d ≥ 3` | `repair` | no |
| desired ⊄ granted | `broaden` | yes |
| expired/expiring (≤7d) + `refresh_capable` + refresh ref | `refresh` | no |
| expired, no refresh | `reauth` | yes |
| healthy + scopes ok | `noop` | no |

Broaden takes precedence over refresh (you need a consent screen anyway).

## Recipe - learned on first success (data, not code)

No recipes ship pre-built. On the first successful setup of a target, write
`recipes/<target>.yaml` capturing last-known-good nav so the next run is faster. Nav steps
are **hints, not coordinates** - if a selector misses (`matches_n: 0`), re-derive live from
`state`/`find`/`extract` and propose a recipe update.
```yaml
target: halopsa
kind: dom_api_key          # dom_api_key (Lane B) | oauth2_pkce_cli (Lane A) | user_paste (Lane C)
scopes: ["tickets read", "assets read"]
destination: {store: credential-store, account: halopsa, service: HALOPSA_API_KEY, wrapper: halopsa-cli}
consumer: {install_check: "command -v halopsa-cli", login_cmd: null, verify_cmd: "halopsa-cli account get", verify_field: ".email"}
nav:
  - {action: open,  url: "https://<your-tenant>.halopsa.com/config/integrations/api"}
  - {action: wait,  type: selector, value: "table"}
  - {action: click, target: "Generate a new API key"}
secret_source: {method: dom_eval, selector: "input#api-key-value[readonly]", attr: value}
holds: ["Delete key", "Revoke"]      # pass to guard_click.py as HOLD_EXTRA for this run
```

Only the three implemented lanes are valid `kind` values. Cookie capture and downloaded
service-account JSON files have no audited no-context lane yet, so do not invent a recipe
for them: fall back to Lane C (the user handles the file or the paste themselves).

## Audit events - `runs/<target>-<ts>/events.jsonl` (structured for three jobs)

`scripts/audit_log.py` appends one JSON event per line; refuses secret-shaped field names
AND values, and locks the file so concurrent writers cannot interleave.
Serves **troubleshooting** (phase/operation/status/error_class/detail), **review** (ordered
stream of what did/didn't happen, holds + resolutions), **improvement** (`patterns.py` mines
`error_class` across runs → proposed global lessons).
```bash
RUN_DIR=<run-dir> uv run scripts/audit_log.py event=consent_shown status=ok target=ninjaone \
   scheme=oauth2 phase=capture operation=broaden selector_id=authorize-btn
```
Suggested keys: `ts run_id target scheme phase operation event status error_class detail
selector_id`; plus `receipt_len receipt_sha8 receipt_last4` (never the value) and
`hold_verb hold_resolved`. Companions in the run dir: `STATE.md` (compaction-survivable
checklist + redacted creds table + holds) and `REPORT.md` (final human summary).
