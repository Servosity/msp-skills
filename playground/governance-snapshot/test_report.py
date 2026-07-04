#!/usr/bin/env python3
"""Runnable check for report.py - the smallest thing that fails if the parser
or renderer breaks. No framework; just asserts. Run: python3 test_report.py"""
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPORT = HERE / "report.py"


def run(*args):
    return subprocess.run([sys.executable, str(REPORT), *args],
                          capture_output=True, text=True)


def main():
    # 1. Demo renders and carries the computed headline + a known finding.
    r = run("--demo", "--date", "2026-07-03")
    assert r.returncode == 0, r.stderr
    assert "third-party apps need review" in r.stdout, "missing exec-summary verdict"
    assert "Contoso Migration Tool" in r.stdout, "missing top finding"
    assert "privilege-escalation" in r.stdout, "missing escalation flag"

    # 2. Sanitize hides vendor names but keeps the risk structure.
    r = run("--demo", "--sanitize", "--date", "2026-07-03")
    assert r.returncode == 0, r.stderr
    assert "Contoso Migration Tool" not in r.stdout, "sanitize leaked a vendor name"
    assert "Third-party app 1" in r.stdout, "sanitize dropped the app rows"
    assert "privilege-escalation" in r.stdout, "sanitize dropped the risk flags"

    # 3. Non-consent JSON is rejected, not silently rendered.
    bad = HERE / "samples" / "_bad.json"
    bad.write_text('{"not":"a consent result"}')
    try:
        r = run(str(bad))
        assert r.returncode != 0, "should reject non-consent JSON"
    finally:
        bad.unlink()

    print("ok: report.py self-check passed")


if __name__ == "__main__":
    main()
