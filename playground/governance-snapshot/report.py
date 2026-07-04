#!/usr/bin/env python3
"""governance-snapshot report engine.

Turns the read-only `microsoft-graph-cli apps consent --json` output into a
white-labelable third-party-app-consent governance report (Markdown).

  ./report.py --demo                          # render the bundled synthetic sample
  ./report.py audit.json                      # render a real audit JSON
  ./report.py audit.json --org "Acme Dental"  # white-label the header
  ./report.py --demo --sanitize               # redact vendor names (shareable)

stdlib only. No network. The input is whatever `apps consent --json` emits, so
demo-mode and real-mode run the identical renderer - the only difference is the
JSON on the way in. That is the whole point: the report cannot be canned.
"""
import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
DEMO_SAMPLE = HERE / "samples" / "demo-consent-audit.json"

# One-line, action-first meaning for each risk flag the audit emits.
FLAG_MEANING = {
    "privilege-escalation": "Holds an application permission that can grant itself or others more access (Application.ReadWrite.All, RoleManagement.ReadWrite.Directory, AppRoleAssignment.ReadWrite.All, or Directory.ReadWrite.All). Treat as tenant-admin-equivalent.",
    "application-permissions": "Runs as itself with NO signed-in user (app-only). App-only access is tenant-wide and unattended - the highest-trust consent class.",
    "high-privilege-delegated": "Delegated consent includes broad read-all, any write-all, mail, or directory/role scopes.",
    "admin-consented": "Tenant-wide consent (AllPrincipals) - applies to every user, not just whoever clicked.",
    "user-consented": "Individual users consented this app themselves (shadow IT). Review your user-consent policy.",
    "disabled-but-consented": "The service principal is disabled yet still carries consent - stale access that should be cleaned up.",
}

# What to do about each flag, most urgent first.
ACTIONS = [
    ("privilege-escalation", "Revoke now or confirm it is a sanctioned admin tool. This permission is a standing path to full tenant control."),
    ("application-permissions", "Confirm each app-only permission is still needed; app-only access does not expire with a user and is easy to forget."),
    ("high-privilege-delegated", "Right-size the delegated scopes to least privilege; drop any read-all/write-all the vendor does not actually use."),
    ("user-consented", "Turn off end-user consent (or restrict it to verified publishers) so new shadow-IT apps require admin review."),
    ("disabled-but-consented", "Remove the consent grants for disabled apps - dead service principals should not retain access."),
]


def load(path):
    with open(path) as f:
        data = json.load(f)
    # Tolerate an --agent/provenance envelope ({"results": {...}}) as well as
    # the raw --json object. apps consent --json emits the raw object.
    if isinstance(data, dict) and "summary" not in data and "results" in data:
        data = data["results"]
    if not isinstance(data, dict) or "summary" not in data or "apps" not in data:
        sys.exit("error: input is not an `apps consent` result (expected keys: summary, apps). "
                 "Generate it with: microsoft-graph-cli apps consent --json")
    return data


def sanitize(data):
    """Replace vendor identities with generic labels; keep every risk signal.
    Produces a shareable sample that leaks no tenant vendor names."""
    out = json.loads(json.dumps(data))  # deep copy
    for i, app in enumerate(out["apps"], 1):
        app["displayName"] = f"Third-party app {i}"
        app.pop("appId", None)
        app["id"] = f"redacted-{i:04d}"
    return out


def fmt_perms(app):
    perms = app.get("applicationPermissions") or []
    if not perms:
        return "-"
    parts = []
    for p in perms:
        name = p.get("permission") or f"appRoleId {p.get('appRoleId', '?')[:8]}..."
        tag = " ⚠escalation" if p.get("escalation") else ""
        parts.append(f"{name} ({p.get('resource', '?')}){tag}")
    return "; ".join(parts)


def fmt_scopes(app):
    hi = app.get("highPrivilegeScopes") or []
    allsc = app.get("delegatedScopes") or []
    if hi:
        extra = len(allsc) - len(hi)
        s = ", ".join(hi)
        return f"**{s}**" + (f" (+{extra} more)" if extra > 0 else "")
    if allsc:
        return ", ".join(allsc[:4]) + (" ..." if len(allsc) > 4 else "")
    return "-"


def render(data, org, as_of, is_demo):
    s = data["summary"]
    apps = data["apps"]
    L = []
    title = f"Third-Party App Consent Report - {org}" if org else "Third-Party App Consent Report"
    L.append(f"# {title}")
    L.append("")
    banner = "**DEMO - synthetic sample, not a real tenant.**  " if is_demo else ""
    L.append(f"{banner}_Generated {as_of} · source: `microsoft-graph-cli apps consent` (read-only)_")
    L.append("")

    # Executive summary.
    L.append("## Executive summary")
    L.append("")
    high = s.get("highRiskApps", 0)
    esc = s.get("appsWithEscalationPermissions", 0)
    verdict = (
        f"**{high} of {s.get('thirdPartyApps', 0)} third-party apps need review"
        + (f", including {esc} with privilege-escalation permissions." if esc else ".")
        + "**"
    )
    L.append(verdict)
    L.append("")
    L.append("| Metric | Count |")
    L.append("| --- | ---: |")
    rows = [
        ("Third-party apps consented", s.get("thirdPartyApps", 0)),
        ("&nbsp;&nbsp;of which external (other tenants)", s.get("externalApps", 0)),
        ("&nbsp;&nbsp;of which internal (homegrown)", s.get("internalApps", 0)),
        ("Admin-consented (tenant-wide)", s.get("adminConsentedApps", 0)),
        ("User-consented (shadow IT)", s.get("userConsentedApps", 0)),
        ("Hold application (app-only) permissions", s.get("appsWithApplicationPermissions", 0)),
        ("Hold privilege-escalation permissions", s.get("appsWithEscalationPermissions", 0)),
        ("Disabled but still consented", s.get("disabledAppsWithConsent", 0)),
        ("**High-risk (score ≥ 3)**", f"**{high}**"),
        ("Microsoft first-party apps (excluded from findings)", s.get("microsoftFirstParty", 0)),
        ("Total service principals scanned", s.get("totalServicePrincipals", 0)),
    ]
    for label, val in rows:
        L.append(f"| {label} | {val} |")
    L.append("")

    if data.get("note"):
        L.append(f"> ⚠ {data['note']}")
        L.append("")

    # Findings.
    L.append("## Findings (highest risk first)")
    L.append("")
    if not apps:
        L.append("_No third-party app consents found._")
    else:
        L.append("| # | App | Origin | Risk | Flags | Application permissions | High-privilege delegated scopes |")
        L.append("| ---: | --- | --- | ---: | --- | --- | --- |")
        for i, app in enumerate(apps, 1):
            flags = ", ".join(app.get("riskFlags", [])) or "-"
            L.append(
                f"| {i} | {app.get('displayName', '?')} | {app.get('origin', '?')} | "
                f"{app.get('riskScore', 0)} | {flags} | {fmt_perms(app)} | {fmt_scopes(app)} |"
            )
    L.append("")

    # Recommended actions, only for flags actually present.
    present = set()
    for app in apps:
        present.update(app.get("riskFlags", []))
    todo = [(f, a) for f, a in ACTIONS if f in present]
    if todo:
        L.append("## Recommended actions")
        L.append("")
        for f, a in todo:
            L.append(f"- **{f}** - {a}")
        L.append("")

    # Glossary of the flags that appeared.
    if present:
        L.append("## What the flags mean")
        L.append("")
        for f in [x for x in FLAG_MEANING if x in present]:
            L.append(f"- **{f}**: {FLAG_MEANING[f]}")
        L.append("")

    L.append("---")
    L.append("_Read-only report. Every number above is spot-checkable in Entra admin center → "
             "Identity → Applications → Enterprise applications → Consent and permissions. "
             "Regenerate any time with `microsoft-graph-cli apps consent --json`._")
    L.append("")
    return "\n".join(L)


def main(argv):
    ap = argparse.ArgumentParser(description="Render a third-party app consent governance report.")
    ap.add_argument("input", nargs="?", help="apps-consent JSON file (omit with --demo)")
    ap.add_argument("--demo", action="store_true", help="render the bundled synthetic sample")
    ap.add_argument("--org", default="", help="organization / client name for the header (white-label)")
    ap.add_argument("--sanitize", action="store_true", help="redact vendor names (shareable sample)")
    ap.add_argument("--date", default="", help="override the 'Generated' date (default: today, UTC)")
    ap.add_argument("-o", "--out", help="write to this file instead of stdout")
    args = ap.parse_args(argv)

    if args.demo:
        path = DEMO_SAMPLE
    elif args.input:
        path = Path(args.input)
    else:
        ap.error("provide an input JSON file or --demo")
    if not Path(path).exists():
        sys.exit(f"error: no such file: {path}")

    data = load(path)
    is_demo = args.demo or "demo" in str(path).lower()
    if args.sanitize:
        data = sanitize(data)
    as_of = args.date or datetime.now(timezone.utc).strftime("%Y-%m-%d")
    md = render(data, args.org, as_of, is_demo)

    if args.out:
        Path(args.out).write_text(md)
        print(f"wrote {args.out}")
    else:
        sys.stdout.write(md)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
