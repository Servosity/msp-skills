#!/usr/bin/env python3
"""check_security_gate.py — deterministic, release-blocking security gate.

The load-bearing control against a poisoned fix reaching downstream MSPs (threat T2).
Model-layer defenses (prompt doctrine, the nonce) are ~22%-breakable; this gate is
deterministic and meant to run as a REQUIRED CI check so it can't be bypassed by a
socially-engineered agent.

Always-on deterministic checks (no external tools needed):
  * Dependency policy — any module in go.mod (require OR `replace` target) not in
    tools/maintainer/dep_allowlist.json is P1; a filesystem `replace` is P1
    (anti-typosquat / anti-backdoor / anti-fork-swap). go.sum changes are surfaced.
  * Dangerous Go patterns — disabled TLS verify; shelling out / exec of a variable or
    shell; syscall.Exec/os.StartProcess/plugin.Open; cgo; //go:generate; //go:linkname;
    import "unsafe". (cobratree MCP→CLI bridge excluded; browser-open literal allowed.)
  * Install-script patterns — pipe-to-shell, eval, base64-decode-then-exec, direct shell
    invocation (cmd /c, & pwsh -Command, mshta/wscript...), non-GitHub download host in
    skills/<slug>/install.sh|.ps1 (the artifact MSPs run via curl|sh). NOTE: an installer
    is executable by nature, so this flags KNOWN-BAD remote/shell-exec forms — it is not a
    complete sandbox. CODEOWNERS review of install-script changes is the human backstop.

Suppressions are BASE-OWNED, not self-grantable: a finding is suppressed only if its
(file, rule) is listed in tools/maintainer/security_suppressions.json. Inline `// #nosec`
is NOT honored (an attacker could add it in their own diff).

Folded in when installed (CI installs them): gosec, govulncheck, osv-scanner, semgrep.

Usage:
    check_security_gate.py --slug ninjaone --base origin/main   # gate the diff
    check_security_gate.py --all                                 # audit every connector
    check_security_gate.py --rebuild-allowlist                   # seed dep_allowlist.json
    check_security_gate.py --slug ninjaone --json

Exit code: 0 = pass (no P1), 1 = block (>=1 P1), 2 = usage error.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
ALLOWLIST = Path(__file__).resolve().parent / "dep_allowlist.json"
SUPPRESSIONS = Path(__file__).resolve().parent / "security_suppressions.json"

BT = "`"  # Go raw-string delimiter; included so backtick evasions are caught.

# --- dangerous Go patterns -----------------------------------------------------
# (id, severity, regex, exclude_path_substr). Calibrated to PASS the healthy fleet
# and BLOCK genuinely dangerous code (false-RED is as useless as false-GREEN).
# Legit press sites NOT flagged: OAuth browser-open (exec.Command("open"/... , url) —
# literal command) and the generated MCP→CLI bridge (internal/mcp/cobratree/).
GO_RULES = [
    ("disabled-tls-verify", "P1",
     re.compile(r"InsecureSkipVerify\s*:\s*true"), None),
    # exec of an interpreter (POSIX or Windows), or passing -c / /C — command injection.
    ("shell-interpreter", "P1",
     re.compile(r'exec\.Command(?:Context)?\([^)]*(["' + BT + r'](?:/bin/|/usr/bin/)?(?:sh|bash|zsh|dash|ash|pwsh(?:\.exe)?|powershell(?:\.exe)?|cmd(?:\.exe)?)["' + BT + r']|,\s*["' + BT + r'](?:-c|/[cC])["' + BT + r'])'),
     None),
    # exec with a NON-literal command (a variable) — the injectable shape.
    ("exec-nonliteral-cmd", "P1",
     re.compile(r'(?:exec\.Command\(\s*[A-Za-z_]|exec\.CommandContext\(\s*[A-Za-z_]\w*\s*,\s*[A-Za-z_])'),
     "/internal/mcp/cobratree/"),
    # exec with a concatenated command (first arg builds a string) — also injectable.
    ("exec-concat-cmd", "P1",
     re.compile(r'(?:exec\.Command\(\s*[^,)]*\+|exec\.CommandContext\([^,]*,\s*[^,)]*\+)'),
     "/internal/mcp/cobratree/"),
    ("syscall-exec", "P1", re.compile(r'\bsyscall\.(?:Exec|ForkExec)\b'), None),
    ("os-startprocess", "P1", re.compile(r'\bos\.StartProcess\b'), None),
    ("plugin-open", "P1", re.compile(r'\bplugin\.Open\b'), None),
    ("cgo-import", "P1", re.compile(r'(?m)^\s*(?:import\s+)?"C"\s*$'), None),
    ("go-generate", "P1", re.compile(r'//go:generate\b'), None),
    ("go-linkname", "P1", re.compile(r'//go:linkname\b'), None),
    ("unsafe-import", "P1", re.compile(r'(?m)^\s*(?:import\s+)?"unsafe"\s*$'), None),
    ("plaintext-http", "P2",
     re.compile(r'"http://(?!localhost|127\.0\.0\.1|0\.0\.0\.1|example\.)'), None),
]

# --- install-script patterns (the artifact downstream MSPs execute via curl|sh / iex) ---
# Case-insensitive: covers POSIX shell AND PowerShell (.ps1) RCE forms.
INSTALL_RULES = [
    ("pipe-to-shell", "P1", re.compile(r'\|\s*(?:sudo\s+)?(?:ba|z|da|a)?sh\b', re.I)),
    ("eval-in-installer", "P1", re.compile(r'(?<![\w-])eval\s', re.I)),
    ("decode-then-run", "P1", re.compile(r'\bbase64\s+(?:-d|--decode|-D)\b|\bfrombase64string\b', re.I)),
    # PowerShell EXECUTION sinks (the dangerous part). Note: Invoke-RestMethod / irm /
    # Invoke-WebRequest to a variable is a benign download (the legit installers query
    # the GitHub releases API that way) — only flag execution: IEX, DownloadString,
    # encoded commands, spawning a shell, or a download PIPED into IEX.
    ("powershell-rce", "P1",
     re.compile(r'\b(?:iex|invoke-expression)\b|\bdownloadstring\b|-e(?:nc\b|ncodedcommand\b)|start-process\b[^\n]{0,60}\b(?:powershell|pwsh|cmd)(?:\.exe)?\b|(?:irm|invoke-restmethod|iwr|invoke-webrequest)\b[^\n|]*\|\s*(?:iex|invoke-expression)', re.I)),
    # Direct shell invocation in an installer. An installer running the connector binary
    # via `& $var` is NOT a shell and won't match; a literal shell/scripting-host does.
    # Covers POSIX (`bash -c`, `/bin/sh -c`, `env sh -c`), Windows cmd with padded flags
    # (`cmd /d /s /c`), powershell with any flags before -Command, the `&` call operator,
    # and scripting hosts. NOTE: an installer is executable by nature — this catches
    # known-bad direct-exec forms; CODEOWNERS review of install-script changes is the
    # backstop for the unbounded tail.
    ("shell-invocation", "P1",
     re.compile(r'\bcmd(?:\.exe)?\b[^\n|]*\s/[ckCK]\b'
                r'|&\s*["\']?(?:powershell|pwsh|cmd|bash|sh|zsh)\b'
                r'|(?:powershell|pwsh)(?:\.exe)?\b[^\n|]*-(?:c|command|e|enc|encodedcommand)\b'
                r'|(?:^|[\s;&|(])(?:env\s+\S*\s+)?(?:/(?:usr/)?bin/)?(?:ba|z|da|a)?sh\s+-c\b'
                r'|\b(?:mshta|wscript|cscript|rundll32)\b', re.I)),
    # download from a non-GitHub host (legit installers pull from GitHub Releases).
    ("nongithub-download", "P2",
     re.compile(r'(?:curl|wget|Invoke-WebRequest|iwr|Invoke-RestMethod|irm)\b[^\n|]*https?://(?!(?:[\w.-]+\.)?(?:github\.com|githubusercontent\.com))', re.I)),
]

# Matches both indented require-block entries AND single-line `require X vY`
# (a single-line require was previously invisible to the allowlist — codex P1).
MOD_RE = re.compile(r"(?m)^(?:\s+|require\s+)([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+v\S+")
ARROW_RE = re.compile(r"=>\s+(\S+)")  # `replace X => TARGET [vY]`


def run(cmd, cwd=None, timeout=120):
    try:
        p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        return p.returncode, p.stdout, p.stderr
    except FileNotFoundError:
        return 127, "", "not-installed"
    except subprocess.TimeoutExpired:
        return 124, "", "timeout"


def have(tool: str) -> bool:
    return run([tool, "--version"])[0] in (0, 1)


def list_slugs() -> list[str]:
    d = REPO / "skills"
    return sorted(p.name for p in d.iterdir() if (p / "cli" / "go.mod").is_file()) if d.is_dir() else []


def modules_in_gomod(gomod: Path) -> set[str]:
    return set(MOD_RE.findall(gomod.read_text(errors="replace"))) if gomod.is_file() else set()


def replace_targets(gomod: Path) -> list[str]:
    return ARROW_RE.findall(gomod.read_text(errors="replace")) if gomod.is_file() else []


def rebuild_allowlist() -> int:
    mods: set[str] = set()
    for slug in list_slugs():
        mods |= modules_in_gomod(REPO / "skills" / slug / "cli" / "go.mod")
    ALLOWLIST.write_text(json.dumps({
        "_comment": ("Approved Go module dependencies for msp-skills connectors. A module in "
                     "any skills/*/cli/go.mod (require OR `replace` target) NOT listed here is a "
                     "P1 in check_security_gate.py — review every addition (anti-typosquat / "
                     "anti-backdoor / anti-fork-swap). Regenerate with --rebuild-allowlist ONLY "
                     "after a human has vetted the new dependency."),
        "allowed": sorted(mods),
    }, indent=2) + "\n")
    print(f"wrote {ALLOWLIST} with {len(mods)} approved modules")
    return 0


def _policy_text(path: Path, ref: str | None) -> str | None:
    """Read a policy file from a trusted base ref (codex P1: in-PR policy edits must not
    take effect), falling back to the working tree when no ref is given."""
    if ref:
        rel = str(path.relative_to(REPO))
        code, out, _ = run(["git", "-C", str(REPO), "show", f"{ref}:{rel}"])
        return out if code == 0 else None
    return path.read_text() if path.is_file() else None


def load_allowlist(ref: str | None = None) -> set[str]:
    txt = _policy_text(ALLOWLIST, ref)
    try:
        return set(json.loads(txt).get("allowed", [])) if txt else set()
    except json.JSONDecodeError:
        return set()


def load_suppressions(ref: str | None = None) -> set[tuple[str, str]]:
    txt = _policy_text(SUPPRESSIONS, ref)
    try:
        return {(e["file"], e["rule"]) for e in json.loads(txt).get("suppress", [])} if txt else set()
    except (json.JSONDecodeError, KeyError):
        return set()


def changed_files(base: str, slug: str) -> list[Path] | None:
    code, out, _ = run(["git", "-C", str(REPO), "diff", "--name-only", f"{base}...HEAD"])
    if code != 0:
        code, out, _ = run(["git", "-C", str(REPO), "diff", "--name-only", base])
        if code != 0:
            return None
    prefix = f"skills/{slug}/"
    return [REPO / f for f in out.splitlines() if f.startswith(prefix)]


def scan_go_patterns(files: list[Path], suppress: set[tuple[str, str]]) -> list[dict]:
    findings = []
    for f in files:
        if f.suffix != ".go" or f.name.endswith("_test.go") or not f.is_file():
            continue
        try:
            text = f.read_text(errors="replace")
        except OSError:
            continue
        rel = str(f.relative_to(REPO))
        for rule, sev, rx, exclude in GO_RULES:
            if exclude and exclude in f"/{rel}":
                continue
            if (rel, rule) in suppress:  # base-owned suppression (NOT self-grantable)
                continue
            m = rx.search(text)
            if m:
                lineno = text[:m.start()].count("\n") + 1
                findings.append({"tool": "builtin", "rule": rule, "severity": sev,
                                 "file": rel, "line": lineno,
                                 "evidence": text[m.start():m.start() + 80].replace("\n", "\\n")})
    return findings


def scan_install_scripts(slug: str, only: list[Path] | None, suppress: set[tuple[str, str]]) -> list[dict]:
    base = REPO / "skills" / slug
    scripts = [base / "install.sh", base / "install.ps1"]
    if only is not None:
        scripts = [s for s in scripts if s in only]
    findings = []
    for s in scripts:
        if not s.is_file():
            continue
        rel = str(s.relative_to(REPO))
        text = s.read_text(errors="replace")
        for rule, sev, rx in INSTALL_RULES:
            if (rel, rule) in suppress:
                continue
            m = rx.search(text)
            if m:
                lineno = text[:m.start()].count("\n") + 1
                findings.append({"tool": "install", "rule": rule, "severity": sev,
                                 "file": rel, "line": lineno,
                                 "evidence": text[m.start():m.start() + 80].replace("\n", "\\n")})
    return findings


def scan_dependencies(slug: str, allow: set[str], only: list[Path] | None) -> list[dict]:
    gomod = REPO / "skills" / slug / "cli" / "go.mod"
    gosum = REPO / "skills" / slug / "cli" / "go.sum"
    findings = []
    if only is not None:
        touched_mod = gomod in only
        touched_sum = gosum in only
        if not touched_mod and not touched_sum:
            return []
        if touched_sum and not touched_mod:
            findings.append({"tool": "dep-allowlist", "rule": "gosum-changed-review", "severity": "P2",
                             "file": f"skills/{slug}/cli/go.sum", "line": 0,
                             "evidence": "go.sum changed without go.mod — review for a swapped checksum."})
        if touched_mod:
            findings.append({"tool": "dep-allowlist", "rule": "gomod-version-review", "severity": "P2",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": "go.mod changed — review every version bump (an allowlisted module "
                                         "bumped to a malicious tag still passes the path check)."})
    # require-block modules not in the allowlist
    for mod in sorted(modules_in_gomod(gomod) - allow):
        findings.append({"tool": "dep-allowlist", "rule": "unapproved-dependency", "severity": "P1",
                         "file": f"skills/{slug}/cli/go.mod", "line": 0,
                         "evidence": f"{mod} is not in dep_allowlist.json — vet it (typosquat/backdoor?)."})
    # replace directives: filesystem target, or a module target not allowlisted
    for tgt in replace_targets(gomod):
        if tgt.startswith((".", "/", "..")):
            findings.append({"tool": "dep-allowlist", "rule": "replace-to-local-path", "severity": "P1",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": f"replace => {tgt} points at a filesystem path (untrusted code swap)."})
        elif "." in tgt.split("/")[0] and tgt not in allow:
            findings.append({"tool": "dep-allowlist", "rule": "replace-to-unapproved", "severity": "P1",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": f"replace => {tgt} swaps in a module not in dep_allowlist.json (fork/backdoor?)."})
    return findings


def scan_external(slug: str) -> list[dict]:
    findings = []
    mod = REPO / "skills" / slug / "cli"
    if have("gosec"):
        _, out, _ = run(["gosec", "-quiet", "-fmt", "json", "./..."], cwd=str(mod))
        try:
            for iss in json.loads(out or "{}").get("Issues", []):
                sev = "P1" if iss.get("severity", "").upper() in ("HIGH", "MEDIUM") else "P2"
                findings.append({"tool": "gosec", "rule": iss.get("rule_id"), "severity": sev,
                                 "file": iss.get("file", ""), "line": iss.get("line", 0),
                                 "evidence": iss.get("details", "")[:120]})
        except json.JSONDecodeError:
            pass
    if have("govulncheck"):
        _, out, _ = run(["govulncheck", "-format", "json", "./..."], cwd=str(mod))
        if '"osv"' in out or "vulnerability" in out.lower():
            findings.append({"tool": "govulncheck", "rule": "known-vuln", "severity": "P1",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": "govulncheck reported a reachable vulnerability."})
    if have("osv-scanner"):
        _, out, _ = run(["osv-scanner", "--lockfile", "go.mod", "--format", "json"], cwd=str(mod))
        try:
            for res in json.loads(out or "{}").get("results", []):
                for pkg in res.get("packages", []):
                    for v in pkg.get("vulnerabilities", []):
                        findings.append({"tool": "osv-scanner", "rule": v.get("id", "OSV"), "severity": "P1",
                                         "file": f"skills/{slug}/cli/go.mod", "line": 0,
                                         "evidence": (v.get("summary", "") or "known vulnerability")[:120]})
        except json.JSONDecodeError:
            pass
    cfg = REPO / ".semgrep.yml"
    if have("semgrep") and cfg.is_file():
        _, out, _ = run(["semgrep", "--config", str(cfg), "--json", "--quiet", str(mod)])
        try:
            for r in json.loads(out or "{}").get("results", []):
                findings.append({"tool": "semgrep", "rule": r.get("check_id"), "severity": "P1",
                                 "file": r.get("path", ""), "line": r.get("start", {}).get("line", 0),
                                 "evidence": (r.get("extra", {}).get("message", ""))[:120]})
        except json.JSONDecodeError:
            pass
    return findings


def go_files(slug: str, only: list[Path] | None) -> list[Path]:
    if only is not None:
        return only
    return list((REPO / "skills" / slug / "cli").rglob("*.go"))


def gate_slug(slug: str, base: str | None, allow: set[str], suppress: set[tuple[str, str]]) -> dict:
    only = changed_files(base, slug) if base else None
    findings = []
    findings += scan_go_patterns(go_files(slug, only), suppress)
    findings += scan_install_scripts(slug, only, suppress)
    findings += scan_dependencies(slug, allow, only)
    findings += scan_external(slug)
    p1 = [f for f in findings if f["severity"] == "P1"]
    return {"slug": slug, "base": base, "scope": "diff" if only is not None else "full",
            "findings": findings, "p1_count": len(p1),
            "verdict": "block" if p1 else "pass",
            "tools": {t: have(t) for t in ("gosec", "govulncheck", "osv-scanner", "semgrep")}}


def main() -> int:
    ap = argparse.ArgumentParser(description="Deterministic release-blocking security gate.")
    ap.add_argument("--slug")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--base", help="git ref to diff against (e.g. origin/main); omit for full audit")
    ap.add_argument("--policy-base", help="read dep_allowlist.json + security_suppressions.json from "
                                          "this trusted ref instead of the (PR-controlled) working tree")
    ap.add_argument("--rebuild-allowlist", action="store_true")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    if args.rebuild_allowlist:
        return rebuild_allowlist()

    allow = load_allowlist(args.policy_base)
    suppress = load_suppressions(args.policy_base)
    if not allow:
        print("WARNING: dep_allowlist.json empty/missing — run --rebuild-allowlist (after vetting).",
              file=sys.stderr)

    slugs = list_slugs() if args.all else ([args.slug] if args.slug else [])
    if not slugs:
        ap.error("one of --slug, --all, or --rebuild-allowlist is required")

    results = [gate_slug(s, args.base, allow, suppress) for s in slugs]
    blocked = [r for r in results if r["verdict"] == "block"]

    if args.json:
        print(json.dumps(results if args.all else results[0], indent=2))
    else:
        for r in results:
            print(f"[{'BLOCK' if r['verdict'] == 'block' else 'pass'}] {r['slug']} ({r['scope']}): "
                  f"{r['p1_count']} P1 / {len(r['findings'])} findings")
            for f in r["findings"]:
                print(f"    {f['severity']} {f['tool']}:{f['rule']} {f['file']}:{f['line']} — {f['evidence']}")
        missing = [t for t, ok in results[0]["tools"].items() if not ok] if results else []
        if missing:
            print(f"\nNOTE: external scanners not installed locally: {', '.join(missing)} "
                  "(CI installs them; built-in deterministic checks still ran).")

    return 1 if blocked else 0


if __name__ == "__main__":
    raise SystemExit(main())
