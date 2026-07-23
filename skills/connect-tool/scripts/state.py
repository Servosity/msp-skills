# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Per-target auth state for connect-tool (idempotency substrate).

Append-only JSONL, read newest-per-target. Holds NO secret
values, only references (credential-store service/account, granted scopes, expiry,
app ids). Two logs in the platform state dir (see ctplatform.state_dir):
  targets.jsonl       - one line per auth-state change; current = last per target
  setup-journal.jsonl - multi-step progress so a partial failure resumes mid-flow

CLI:
  uv run state.py append '<json>'         # append a target entry (json arg or stdin)
  uv run state.py current [TARGET]        # newest-per-target, as a json object/list
  uv run state.py journal-append '<json>' # append a setup-journal step
  uv run state.py journal TARGET          # newest journal entry for TARGET
  uv run state.py --selfcheck
"""
from __future__ import annotations

import datetime as _dt
import json
import re
import sys

import ctplatform as ct

# Fields that must NEVER appear in state (defense in depth - state holds refs, not values).
_FORBIDDEN = {"value", "secret", "token", "access_token", "refresh_token",
              "api_key", "client_secret", "bearer", "password"}
# Catch a secret embedded inside an otherwise-innocent value (e.g. detail='{"access_token":"x"}').
_EMBEDDED = re.compile(r'(access_token|refresh_token|client_secret|api_?key|password|passwd)["\s]*[:=]', re.I)


def state_dir():
    """Platform-correct, private state dir (ctplatform owns the seam)."""
    return ct.state_dir()


def now_iso() -> str:
    return _dt.datetime.now(_dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _reject_secret_keys(rec: dict) -> None:
    bad = _FORBIDDEN & {k.lower() for k in rec}
    if bad:
        raise SystemExit(f"refusing to persist forbidden secret field(s): {sorted(bad)}")


def _reject_secret_values(rec: dict) -> None:
    for k, v in rec.items():
        blob = v if isinstance(v, str) else (json.dumps(v) if isinstance(v, (list, dict)) else "")
        if blob and _EMBEDDED.search(blob):
            raise SystemExit(f"refusing to persist value with an embedded secret in field '{k}'")


def _append(fname: str, rec: dict) -> dict:
    _reject_secret_keys(rec)
    _reject_secret_values(rec)
    rec.setdefault("ts", now_iso())
    with (state_dir() / fname).open("a") as fh:
        fh.write(json.dumps(rec, sort_keys=True) + "\n")
    return rec


def _read(fname: str) -> list[dict]:
    p = state_dir() / fname
    if not p.exists():
        return []
    out = []
    for line in p.read_text().splitlines():
        line = line.strip()
        if line:
            out.append(json.loads(line))
    return out


def append_target(rec: dict) -> dict:
    if "target" not in rec:
        raise SystemExit("target entry requires a 'target' field")
    return _append("targets.jsonl", rec)


# Older entries used macOS-specific names before Windows support existed.
_LEGACY_REFS = {
    "access_token_keychain_ref": "access_token_credential_ref",
    "refresh_token_keychain_ref": "refresh_token_credential_ref",
    "client_secret_keychain_ref": "client_secret_credential_ref",
}


def migrate(rec: dict) -> dict:
    """Read-time rename of the *_keychain_ref fields. Nothing is rewritten on
    disk: the log is append-only, and old lines stay readable forever."""
    for old, new in _LEGACY_REFS.items():
        if old in rec and new not in rec:
            rec[new] = rec.pop(old)
    return rec


def latest() -> dict[str, dict]:
    """Newest entry per target (last line wins)."""
    out: dict[str, dict] = {}
    for rec in _read("targets.jsonl"):
        t = rec.get("target")
        if t:
            out[t] = migrate(rec)
    return out


def current(target: str | None = None):
    """One target's newest entry if target given, else the full newest-per-target map."""
    m = latest()
    return m.get(target) if target is not None else m


def journal_append(rec: dict) -> dict:
    if "target" not in rec or "step" not in rec:
        raise SystemExit("journal entry requires 'target' and 'step'")
    return _append("setup-journal.jsonl", rec)


def journal(target: str) -> dict | None:
    last = None
    for rec in _read("setup-journal.jsonl"):
        if rec.get("target") == target:
            last = rec
    return last


def _selfcheck() -> None:
    import os
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        os.environ["CONNECT_TOOL_STATE_DIR"] = td
        append_target({"target": "demo", "status": "app_ready", "scopes_granted": []})
        append_target({"target": "demo", "status": "authenticated",
                       "scopes_granted": ["read"],
                       "access_token_credential_ref": "connect-tool-demo-token"})
        cur = current("demo")
        assert cur is not None and cur["status"] == "authenticated", cur
        assert cur["scopes_granted"] == ["read"], cur
        assert latest()["demo"]["status"] == "authenticated"
        journal_append({"target": "demo", "step": "app_created", "status": "ok"})
        journal_append({"target": "demo", "step": "token_stored", "status": "ok"})
        j = journal("demo")
        assert j is not None and j["step"] == "token_stored"
        # forbidden-field guard (key name is a secret)
        try:
            append_target({"target": "x", "access_token": "leak"})
            raise AssertionError("forbidden field was not rejected")
        except SystemExit:
            pass
        # embedded-secret VALUE guard (secret hidden under an innocent key)
        try:
            append_target({"target": "y", "detail": '{"access_token":"leak"}'})
            raise AssertionError("embedded-secret value was not rejected")
        except SystemExit:
            pass
        # benign value mentioning "token" must NOT trip (no false positive)
        append_target({"target": "z", "note": "token refresh scheduled"})
        # a legacy *_keychain_ref entry still reads, under the new name
        append_target({"target": "legacy", "status": "authenticated",
                       "refresh_token_keychain_ref": "old-style-ref"})
        leg = current("legacy")
        assert leg["refresh_token_credential_ref"] == "old-style-ref", leg
        assert "refresh_token_keychain_ref" not in leg, leg
    print("state.py selfcheck OK")


def main(argv: list[str]) -> int:
    if not argv or argv[0] in ("-h", "--help"):
        print(__doc__)
        return 0
    cmd = argv[0]
    if cmd == "--selfcheck":
        _selfcheck()
        return 0
    if cmd == "append":
        rec = json.loads(argv[1] if len(argv) > 1 else sys.stdin.read())
        print(json.dumps(append_target(rec)))
        return 0
    if cmd == "current":
        print(json.dumps(current(argv[1] if len(argv) > 1 else None)))
        return 0
    if cmd == "journal-append":
        rec = json.loads(argv[1] if len(argv) > 1 else sys.stdin.read())
        print(json.dumps(journal_append(rec)))
        return 0
    if cmd == "journal":
        print(json.dumps(journal(argv[1])))
        return 0
    print(f"unknown command: {cmd}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
