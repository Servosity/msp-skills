# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Reconcile desired auth state vs current → the minimal operation.

Terraform-for-auth: given a desired spec (target + desired scopes) and the
current per-target state (from state.py), emit ONE operation. "Already set up"
is never a dead end - broadening a scope or refreshing a token are first-class.

  uv run reconcile.py --target halopsa --scopes contacts_read,contacts_write
  uv run reconcile.py --target halopsa --scopes read --current '<json>'  # explicit state
  uv run reconcile.py --selfcheck

Output (stdout JSON): {operation, reason, browser, missing_scopes}
operations: setup | noop | refresh | broaden | reauth | repair
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import sys

try:
    import state as _state  # type: ignore[reportMissingImports]  # sibling module, on sys.path at runtime
except ImportError:  # pragma: no cover - when invoked from elsewhere
    _state = None

REFRESH_WINDOW_DAYS = 7
ERROR_RESET_THRESHOLD = 3


def _parse_ts(ts: str | None) -> _dt.datetime | None:
    if not ts:
        return None
    return _dt.datetime.strptime(ts, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=_dt.timezone.utc)


def reconcile(desired_scopes: list[str], current: dict | None,
              now: _dt.datetime, window_days: int = REFRESH_WINDOW_DAYS) -> dict:
    desired = set(desired_scopes)

    if not current:
        return {"operation": "setup", "reason": "no prior state for target",
                "browser": True, "missing_scopes": sorted(desired)}

    if int(current.get("error_count_7d", 0)) >= ERROR_RESET_THRESHOLD:
        return {"operation": "repair", "reason": "error_count_7d >= 3; surface and suggest reset",
                "browser": False, "missing_scopes": []}

    granted = set(current.get("scopes_granted") or [])
    missing = sorted(desired - granted)

    # Broadening needs a consent screen regardless of token freshness - do it first.
    if missing:
        return {"operation": "broaden", "reason": f"desired scopes not granted: {missing}",
                "browser": True, "missing_scopes": missing}

    expiry = _parse_ts(current.get("token_expiry_ts"))
    refresh_capable = bool(current.get("refresh_capable"))
    has_refresh = bool(current.get("refresh_token_credential_ref"))
    status = current.get("status", "")

    expired_or_soon = expiry is not None and now >= (expiry - _dt.timedelta(days=window_days))

    if status == "token_expired" or expired_or_soon:
        if refresh_capable and has_refresh:
            return {"operation": "refresh", "reason": "token expired/expiring and refresh available",
                    "browser": False, "missing_scopes": []}
        return {"operation": "reauth", "reason": "token expired and no refresh token",
                "browser": True, "missing_scopes": []}

    # noop ONLY for a target that actually reached `authenticated`. A half-finished
    # setup (app_ready, auth_error, ...) has satisfied scopes trivially and must not
    # short-circuit into "nothing to do".
    if status != "authenticated":
        return {"operation": "setup", "reason": f"prior state is {status or 'incomplete'}, not authenticated",
                "browser": True, "missing_scopes": sorted(desired)}

    return {"operation": "noop", "reason": "authenticated, scopes satisfied and token valid",
            "browser": False, "missing_scopes": []}


def _selfcheck() -> None:
    now = _dt.datetime(2026, 6, 29, tzinfo=_dt.timezone.utc)
    soon = (now + _dt.timedelta(days=3)).strftime("%Y-%m-%dT%H:%M:%SZ")
    far = (now + _dt.timedelta(days=90)).strftime("%Y-%m-%dT%H:%M:%SZ")
    cases = [
        ("setup", ["read"], None),
        ("noop", ["read"], {"status": "authenticated", "scopes_granted": ["read"], "token_expiry_ts": far}),
        ("noop", ["read"], {"status": "authenticated", "scopes_granted": ["read"], "token_expiry_ts": None}),
        ("broaden", ["read", "write"], {"status": "authenticated", "scopes_granted": ["read"], "token_expiry_ts": far}),
        ("refresh", ["read"], {"status": "authenticated", "scopes_granted": ["read"], "token_expiry_ts": soon,
                               "refresh_capable": True, "refresh_token_credential_ref": "connect-tool-x-refresh"}),
        ("reauth", ["read"], {"status": "token_expired", "scopes_granted": ["read"], "token_expiry_ts": soon}),
        ("repair", ["read"], {"status": "authenticated", "scopes_granted": ["read"],
                              "token_expiry_ts": far, "error_count_7d": 4}),
        # a half-finished setup must NOT read as noop just because nothing is missing
        ("setup", [], {"status": "app_ready", "scopes_granted": [], "token_expiry_ts": None}),
        ("setup", ["read"], {"status": "auth_error", "scopes_granted": ["read"], "token_expiry_ts": far}),
    ]
    for want, scopes, cur in cases:
        got = reconcile(scopes, cur, now)["operation"]
        assert got == want, f"expected {want}, got {got} for {cur}"
    print("reconcile.py selfcheck OK (setup/noop/broaden/refresh/reauth/repair, incomplete-state guard)")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--target")
    ap.add_argument("--scopes", default="", help="comma-separated desired scopes")
    ap.add_argument("--current", help="explicit current-state JSON (else read via state.py)")
    ap.add_argument("--now", help="ISO override for testing")
    ap.add_argument("--window-days", type=int, default=REFRESH_WINDOW_DAYS)
    ap.add_argument("--selfcheck", action="store_true")
    args = ap.parse_args(argv)

    if args.selfcheck:
        _selfcheck()
        return 0
    if not args.target:
        ap.error("--target is required (or use --selfcheck)")

    scopes = [s for s in (args.scopes.split(",") if args.scopes else []) if s]
    if args.current is not None:
        current = json.loads(args.current) if args.current else None
    elif _state is not None:
        current = _state.current(args.target)
    else:
        current = None
    now = _parse_ts(args.now) or _dt.datetime.now(_dt.timezone.utc)
    print(json.dumps(reconcile(scopes, current, now, args.window_days)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
