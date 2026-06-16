#!/usr/bin/env python3
"""check_security_gate.py - deterministic, release-blocking security gate.

The load-bearing control against a poisoned fix reaching downstream MSPs (threat T2).
Model-layer defenses (prompt doctrine, the nonce) are ~22%-breakable; this gate is
deterministic and meant to run as a REQUIRED CI check so it can't be bypassed by a
socially-engineered agent.

Always-on deterministic checks (no external tools needed):
  * Dependency policy - any module in go.mod (require OR `replace` target) not in
    tools/maintainer/dep_allowlist.json is P1; a filesystem `replace` is P1
    (anti-typosquat / anti-backdoor / anti-fork-swap). go.sum changes are surfaced.
  * Dangerous Go patterns - disabled TLS verify; shelling out / exec of a variable or
    shell; syscall.Exec/os.StartProcess/plugin.Open; cgo; //go:generate; //go:linkname;
    import "unsafe". (cobratree MCP→CLI bridge excluded; browser-open literal allowed.)
  * Install-script patterns - pipe-to-shell, eval, base64-decode-then-exec, direct shell
    invocation (cmd /c, & pwsh -Command, mshta/wscript...), non-GitHub download host in
    skills/<slug>/install.sh|.ps1 (the artifact MSPs run via curl|sh). NOTE: an installer
    is executable by nature, so this flags KNOWN-BAD remote/shell-exec forms - it is not a
    complete sandbox. CODEOWNERS review of install-script changes is the human backstop.

Suppressions are BASE-OWNED, not self-grantable: a finding is suppressed only if its
(file, rule) is listed in tools/maintainer/security_suppressions.json. Inline `// #nosec`
is NOT honored (an attacker could add it in their own diff).

Folded in when installed (CI installs them): gosec, govulncheck, osv-scanner, semgrep.
  * govulncheck = the REACHABILITY gate. Run in DEFAULT text/symbol mode: exit 3 = a
    vulnerable symbol is CALLED from this module (P1); 0 = none called. (-format json
    ALWAYS exits 0 regardless of findings, which silently neutered the prior gate.)
  * osv-scanner = PRESENCE-only (no reachability). A bare advisory is a tracked P2; it
    only GATES (P1) when the PR introduced/bumped that module@version, the module is a
    direct first-party import (import-graph, not the `// indirect` comment), or - at
    release only - a GHSA-only high/critical advisory govulncheck's DB cannot reason about.
  * --require-scanners (CI/release): a missing/erroring required scanner is a P1, never a
    silent skip; unresolved osv->module mapping and `go list` failures also fail CLOSED.

Usage:
    check_security_gate.py --slug ninjaone --base origin/main   # gate the diff
    check_security_gate.py --slug ninjaone --base origin/main --require-scanners  # CI
    check_security_gate.py --all                                 # audit every connector
    check_security_gate.py --rebuild-allowlist                   # seed dep_allowlist.json
    check_security_gate.py --slug ninjaone --json

Exit code: 0 = pass (no P1), 1 = block (>=1 P1), 2 = usage error.
"""
from __future__ import annotations

import argparse
import json
import os
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
# Legit press sites NOT flagged: OAuth browser-open (exec.Command("open"/... , url) -
# literal command) and the generated MCP→CLI bridge (internal/mcp/cobratree/).
GO_RULES = [
    ("disabled-tls-verify", "P1",
     re.compile(r"InsecureSkipVerify\s*:\s*true"), None),
    # exec of an interpreter (POSIX or Windows), or passing -c / /C - command injection.
    ("shell-interpreter", "P1",
     re.compile(r'exec\.Command(?:Context)?\([^)]*(["' + BT + r'](?:/bin/|/usr/bin/)?(?:sh|bash|zsh|dash|ash|pwsh(?:\.exe)?|powershell(?:\.exe)?|cmd(?:\.exe)?)["' + BT + r']|,\s*["' + BT + r'](?:-c|/[cC])["' + BT + r'])'),
     None),
    # exec with a NON-literal command (a variable) - the injectable shape.
    ("exec-nonliteral-cmd", "P1",
     re.compile(r'(?:exec\.Command\(\s*[A-Za-z_]|exec\.CommandContext\(\s*[A-Za-z_]\w*\s*,\s*[A-Za-z_])'),
     "/internal/mcp/cobratree/"),
    # exec with a concatenated command (first arg builds a string) - also injectable.
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
    # the GitHub releases API that way) - only flag execution: IEX, DownloadString,
    # encoded commands, spawning a shell, or a download PIPED into IEX.
    ("powershell-rce", "P1",
     re.compile(r'\b(?:iex|invoke-expression)\b|\bdownloadstring\b|-e(?:nc\b|ncodedcommand\b)|start-process\b[^\n]{0,60}\b(?:powershell|pwsh|cmd)(?:\.exe)?\b|(?:irm|invoke-restmethod|iwr|invoke-webrequest)\b[^\n|]*\|\s*(?:iex|invoke-expression)', re.I)),
    # Direct shell invocation in an installer. An installer running the connector binary
    # via `& $var` is NOT a shell and won't match; a literal shell/scripting-host does.
    # Covers POSIX (`bash -c`, `/bin/sh -c`, `env sh -c`), Windows cmd with padded flags
    # (`cmd /d /s /c`), powershell with any flags before -Command, the `&` call operator,
    # and scripting hosts. NOTE: an installer is executable by nature - this catches
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
# (a single-line require was previously invisible to the allowlist - codex P1).
MOD_RE = re.compile(r"(?m)^(?:\s+|require\s+)([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+v\S+")
# Same shape but ALSO captures the version (for go.mod base-vs-HEAD version-delta).
MOD_VER_RE = re.compile(r"(?m)^(?:\s+|require\s+)([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+(v\S+)")
ARROW_RE = re.compile(r"=>\s+(\S+)")  # `replace X => TARGET [vY]`

# Scanners that provide UNIQUE coverage and so are fail-closed under --require-scanners
# (a missing/erroring one is a P1 in CI/release, not a silent skip). semgrep is excluded
# deliberately: its .semgrep.yml rules MIRROR the always-on builtin GO_RULES, so its
# absence does not reduce coverage, and its pip install is the flakiest - making it
# fail-closed would re-introduce the very false-RED this change removes.
REQUIRED_SCANNERS = ("gosec", "govulncheck", "osv-scanner")

# Control-plane rules that report the gate ITSELF failed (a scanner missing/erroring, an
# osv package that won't map). These are NEVER suppressible - a base-owned suppression
# must not be able to silence "the gate could not run" (codex). They are always P1.
CONTROL_RULES = frozenset({"scanner-missing", "scanner-error", "osv-mapping-error"})


def run(cmd, cwd=None, timeout=120, env=None):
    try:
        p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout,
                           env=({**os.environ, **env} if env else None))
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
                     "P1 in check_security_gate.py - review every addition (anti-typosquat / "
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
                             "evidence": "go.sum changed without go.mod - review for a swapped checksum."})
        if touched_mod:
            findings.append({"tool": "dep-allowlist", "rule": "gomod-version-review", "severity": "P2",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": "go.mod changed - review every version bump (an allowlisted module "
                                         "bumped to a malicious tag still passes the path check)."})
    # require-block modules not in the allowlist
    for mod in sorted(modules_in_gomod(gomod) - allow):
        findings.append({"tool": "dep-allowlist", "rule": "unapproved-dependency", "severity": "P1",
                         "file": f"skills/{slug}/cli/go.mod", "line": 0,
                         "evidence": f"{mod} is not in dep_allowlist.json - vet it (typosquat/backdoor?)."})
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


def _external_rel(file_path: str, slug: str) -> tuple[str, str]:
    """Normalize a scanner-reported file path to (repo_relative, slug_relative).

    External scanners (gosec especially) emit ABSOLUTE paths; some emit
    module-relative ones. Return both the repo-relative form
    (skills/<slug>/cli/...) the other scanners use, and a slug-relative form
    (cli/...). A suppression entry may then be written repo-relative (scopes to
    one connector) OR slug-relative (fleet-wide, for the byte-identical
    printing-press generated files every connector carries)."""
    if not file_path:
        return "", ""
    skill_prefix = f"skills/{slug}/"        # slug-relative is relative to the skill dir -> cli/...
    cli_prefix = f"skills/{slug}/cli/"      # reconstruct repo-relative from a module-relative path
    p = Path(file_path)
    if p.is_absolute():
        try:
            repo_rel = str(p.relative_to(REPO))
        except ValueError:
            repo_rel = file_path
    elif file_path.startswith("skills/"):
        repo_rel = file_path
    else:
        repo_rel = cli_prefix + file_path
    slug_rel = repo_rel[len(skill_prefix):] if repo_rel.startswith(skill_prefix) else repo_rel
    return repo_rel, slug_rel


def gomod_requires(text: str) -> dict[str, str]:
    """Parse go.mod require lines into {module_path: version}."""
    return {m.group(1): m.group(2) for m in MOD_VER_RE.finditer(text or "")}


def pr_introduced_modules(slug: str, base: str | None) -> set[str]:
    """Modules ADDED or version-BUMPED by this PR's go.mod diff (base -> HEAD).

    This is the non-gameable osv discriminator: it compares the actual base-vs-HEAD
    module@version set, NOT the attacker-influenceable `// indirect` comment. Empty in
    full-audit/release mode (no base)."""
    if not base:
        return set()
    rel = f"skills/{slug}/cli/go.mod"
    head_path = REPO / rel
    head = gomod_requires(head_path.read_text(errors="replace")) if head_path.is_file() else {}
    code, out, _ = run(["git", "-C", str(REPO), "show", f"{base}:{rel}"])
    base_req = gomod_requires(out) if code == 0 else {}
    return {p for p, v in head.items() if base_req.get(p) != v}


# (GOOS, GOARCH) pairs the release builds (release.yml matrix: 3 OS x 2 arch). A direct
# import behind `//go:build windows` OR `//go:build arm64` (or a `*_windows.go` /
# `*_arm64.go` filename tag) is invisible to a host-only `go list`, so a vulnerable module
# imported only on one target could be wrongly demoted (codex). Union the import graph
# across EVERY shipped target; CGO_ENABLED=0 matches the release build so directness does
# not over-count cgo-only imports absent from the shipped binaries.
BUILD_TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"),
                 ("darwin", "arm64"), ("windows", "amd64"), ("windows", "arm64"))


def _go_list_direct(mod_dir: Path, goos: str, goarch: str) -> set[str] | None:
    code, out, _ = run(["go", "list", "-deps", "-json", "./..."], cwd=str(mod_dir),
                       timeout=300, env={"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})
    if code != 0 or not out:
        return None
    dec, i, pkgs = json.JSONDecoder(), 0, []
    while i < len(out):
        while i < len(out) and out[i] in " \t\r\n":
            i += 1
        if i >= len(out):
            break
        try:
            obj, i = dec.raw_decode(out, i)
        except json.JSONDecodeError:
            return None
        pkgs.append(obj)
    pkg_mod = {p.get("ImportPath", ""): (p.get("Module") or {}).get("Path")
               for p in pkgs if (p.get("Module") or {}).get("Path")}
    direct: set[str] = set()
    for p in pkgs:
        if (p.get("Module") or {}).get("Main"):  # a first-party (main-module) package
            for imp in p.get("Imports", []):
                m = pkg_mod.get(imp)
                if m:
                    direct.add(m)
    return direct


def direct_import_modules(mod_dir: Path) -> set[str] | None:
    """Modules whose packages are imported by THIS module's own first-party code,
    unioned across every release GOOS.

    Computed from the import GRAPH (`go list -deps -json`), NOT the `// indirect`
    comment (codex: the comment is attacker-influenceable). Returns None if `go list`
    fails for ANY target - the caller fails CLOSED (P1 scanner-error) in CI/release
    rather than silently treating unknown directness as non-direct."""
    union: set[str] = set()
    for goos, goarch in BUILD_TARGETS:
        d = _go_list_direct(mod_dir, goos, goarch)
        if d is None:
            return None  # fail closed: a target we cannot resolve = unknown directness
        union |= d
    return union


def _longest_module_prefix(name: str, mods: set[str]) -> str | None:
    """Map an osv package path to the owning go.mod module (longest-prefix, path-aware).
    For Go + `--lockfile go.mod`, osv's package.name is already the module path; this is
    the safety net for subpackage/nested-module cases."""
    cands = [m for m in mods if name == m or name.startswith(m + "/")]
    return max(cands, key=len) if cands else None


def scan_external(slug: str, suppress: set[tuple[str, str]],
                  base: str | None = None, require: bool = False) -> list[dict]:
    findings = []
    mod = REPO / "skills" / slug / "cli"

    # Same base-owned suppression model as scan_go_patterns / scan_install_scripts
    # (the module docstring promises "a finding is suppressed only if its
    # (file, rule) is listed"); previously the external scanners ignored it, so
    # gosec findings were unsuppressable. Now normalized + filtered here.
    def add(tool: str, rule: str, sev: str, file_path: str, line, evidence: str) -> None:
        repo_rel, slug_rel = _external_rel(file_path, slug)
        # Control-plane rules ("the gate could not run") are never suppressible.
        if rule not in CONTROL_RULES and ((repo_rel, rule) in suppress or (slug_rel, rule) in suppress):
            return
        findings.append({"tool": tool, "rule": rule, "severity": sev,
                         "file": repo_rel or file_path, "line": line, "evidence": evidence})

    # Fail-closed (codex): under --require-scanners (CI/release) a missing required
    # scanner is a P1, not a silent skip. A transient install failure or an attacker
    # who can break the install must not be able to disable a gate quietly.
    gomod_path = mod / "go.mod"
    if require:
        for t in REQUIRED_SCANNERS:
            if not have(t):
                add(t, "scanner-missing", "P1", f"skills/{slug}/cli/go.mod", 0,
                    f"required scanner {t} not installed (fail-closed in CI/release).")

    if have("gosec"):
        # gosec exits 0 (clean) or 1 (issues found) on a successful run; exit code alone
        # is ambiguous (1 = issues found), so the "ran" signal is parseable JSON. Fail
        # CLOSED under --require-scanners only when gosec did NOT run: unparseable output,
        # timeout (124), or not-installed (127). We deliberately do NOT treat a non-empty
        # gosec "Golang errors" as a failure: its go/packages loader routinely emits benign
        # per-package errors (e.g. `undefined: <sym>` from command-line-arguments loads) on
        # HEALTHY connectors while still analyzing them (30+ Issues), so gating on it is a
        # fleet-wide false-RED. A genuine build break is still caught by govulncheck
        # (fail-closed) and the always-on builtin pattern scan.
        code, out, err = run(["gosec", "-quiet", "-fmt", "json", "./..."], cwd=str(mod), timeout=300)
        gj = None
        try:
            gj = json.loads(out or "{}")
        except json.JSONDecodeError:
            gj = None
        if (gj is None or code in (124, 127)) and require:
            detail = "timeout" if code == 124 else "not-installed" if code == 127 else "unparseable output"
            add("gosec", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"gosec did not complete ({detail}, exit {code}) (fail-closed).")
        for iss in (gj or {}).get("Issues", []):
            sev = "P1" if iss.get("severity", "").upper() in ("HIGH", "MEDIUM") else "P2"
            add("gosec", iss.get("rule_id"), sev, iss.get("file", ""),
                iss.get("line", 0), iss.get("details", "")[:120])
    if have("govulncheck"):
        # REACHABILITY gate. Run DEFAULT text/symbol mode and use its exit code:
        # 0 = no called vuln, 3 = a vulnerable symbol is CALLED from this module.
        # CRITICAL: -format json/sarif/openvex ALWAYS exit 0 regardless of findings
        # (documented), so the prior `-format json` + `code == 3` was dead code -
        # govulncheck never once fired a P1 (verified: a known-REACHABLE x/text vuln
        # exits 0 under -format json, 3 under text mode). Any OTHER nonzero is a
        # load/build/usage error and fails CLOSED under --require-scanners.
        code, out, err = run(["govulncheck", "./..."], cwd=str(mod), timeout=300)
        if code == 3:
            ev = next((ln.strip() for ln in out.splitlines() if ln.strip().startswith("Vulnerability #")),
                      "govulncheck: a vulnerability is reachable from this module's code.")
            add("govulncheck", "known-vuln", "P1", f"skills/{slug}/cli/go.mod", 0, ev[:120])
        elif code != 0 and require:
            add("govulncheck", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"govulncheck failed (exit {code}): {(err or out).strip()[:90]} (fail-closed).")
    if have("osv-scanner"):
        # osv-scanner is PRESENCE-only (no reachability). Gating on every advisory
        # re-blocks the whole fleet on press-pinned transitive deps a connector PR
        # cannot fix (the false-RED treadmill). Demote presence to a tracked P2;
        # keep P1 only when the PR itself introduced/bumped the module@version, when
        # the module is a DIRECT first-party import (computed from the import graph,
        # not the `// indirect` comment), or - at RELEASE only - a GHSA-only
        # high/critical advisory govulncheck's DB cannot reason about. govulncheck
        # (reachability) and dep-allowlist (identity) remain the independent P1 gates.
        osv_mods = set(gomod_requires(gomod_path.read_text(errors="replace"))) if gomod_path.is_file() else set()
        introduced = pr_introduced_modules(slug, base)
        direct = direct_import_modules(mod)  # None => go list failed (for ANY release GOOS)
        # Fail CLOSED (codex #4) the moment directness is unknowable - independent of
        # whether osv returns any vuln, so a `go list` failure can't be masked by an
        # empty osv result.
        if direct is None and require:
            add("osv-scanner", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                "`go list -deps` failed (a release GOOS did not resolve); cannot compute "
                "import-graph directness (fail-closed).")
        code_o, out, err_o = run(["osv-scanner", "--lockfile", "go.mod", "--format", "json"],
                                 cwd=str(mod), timeout=300)
        results = []
        try:
            results = json.loads(out or "{}").get("results", [])
            parsed_ok = True
        except json.JSONDecodeError:
            parsed_ok = False
        # osv-scanner: exit 0 = clean, 1 = vulnerabilities found (both are a successful
        # run we parse); ANY other code (timeout 124, missing 127, internal error) or
        # unparseable output means osv did not run -> fail CLOSED under --require-scanners
        # (codex #1: a nonzero-with-empty-stdout previously passed open).
        if (code_o not in (0, 1) or not parsed_ok) and require:
            add("osv-scanner", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"osv-scanner did not complete cleanly (exit {code_o}, "
                f"{'unparseable output' if not parsed_ok else (err_o or '').strip()[:60]}) (fail-closed).")
        for res in results:
            for pkg in res.get("packages", []):
                pinfo = pkg.get("package", {})
                pname, pver = pinfo.get("name", ""), pinfo.get("version", "?")
                sevmap = {vid: g.get("max_severity", "")
                          for g in pkg.get("groups", []) for vid in g.get("ids", [])}
                for v in pkg.get("vulnerabilities", []):
                    vid = v.get("id", "OSV")
                    module = pname if pname in osv_mods else _longest_module_prefix(pname, osv_mods)
                    if not module:
                        add("osv-scanner", "osv-mapping-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                            f"{vid}: osv package {pname!r} did not resolve to a go.mod module (fail-closed).")
                        continue
                    if direct is None:
                        # directness unknowable - the gate already blocked above (or is a
                        # local best-effort run); classify conservatively as not-direct so
                        # the P2 line is still recorded for visibility.
                        is_direct = False
                    else:
                        is_direct = module in direct
                    pr_new = module in introduced
                    ids = [vid] + list(v.get("aliases", []))
                    go_covered = any(str(x).startswith("GO-") for x in ids)
                    raw_sev = (sevmap.get(vid, "") or "").strip()
                    try:
                        sev_num = float(raw_sev) if raw_sev else None
                    except ValueError:
                        sev_num = None
                    # Release-only carve-out: an advisory govulncheck's DB cannot reason
                    # about (no GO- id) that is high/critical (or unknown severity) stays
                    # P1 until reviewed. Never fires in PR mode, so no treadmill.
                    ghsa_gate = (base is None) and (not go_covered) and (sev_num is None or sev_num >= 7.0)
                    gating = pr_new or is_direct or ghsa_gate
                    why = ("PR introduced/bumped this module@version" if pr_new else
                           "direct first-party import" if is_direct else
                           "release: GHSA-only, govulncheck-DB-blind, high/unknown severity" if ghsa_gate else
                           "transitive presence-only (tracked; govulncheck gates reachability)")
                    sev_tag = f"[{raw_sev}] " if raw_sev else ""
                    add("osv-scanner", vid, "P1" if gating else "P2", f"skills/{slug}/cli/go.mod", 0,
                        f"{module}@{pver} {sev_tag}- {why}; {(v.get('summary', '') or '')[:70]}")
    cfg = REPO / ".semgrep.yml"
    if have("semgrep") and cfg.is_file():
        _, out, _ = run(["semgrep", "--config", str(cfg), "--json", "--quiet", str(mod)])
        try:
            for r in json.loads(out or "{}").get("results", []):
                add("semgrep", r.get("check_id"), "P1", r.get("path", ""),
                    r.get("start", {}).get("line", 0), (r.get("extra", {}).get("message", ""))[:120])
        except json.JSONDecodeError:
            pass
    return findings


def go_files(slug: str, only: list[Path] | None) -> list[Path]:
    if only is not None:
        return only
    return list((REPO / "skills" / slug / "cli").rglob("*.go"))


def gate_slug(slug: str, base: str | None, allow: set[str], suppress: set[tuple[str, str]],
              require: bool = False) -> dict:
    only = changed_files(base, slug) if base else None
    findings = []
    findings += scan_go_patterns(go_files(slug, only), suppress)
    findings += scan_install_scripts(slug, only, suppress)
    findings += scan_dependencies(slug, allow, only)
    findings += scan_external(slug, suppress, base, require)
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
    ap.add_argument("--require-scanners", action="store_true",
                    help="fail CLOSED (P1) if a required scanner (gosec/govulncheck/osv-scanner) "
                         "is missing or errors. Set this in CI/release so a broken scanner install "
                         "cannot silently disable the gate. Omit for local dev (graceful skip).")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    if args.rebuild_allowlist:
        return rebuild_allowlist()

    allow = load_allowlist(args.policy_base)
    suppress = load_suppressions(args.policy_base)
    if not allow:
        print("WARNING: dep_allowlist.json empty/missing - run --rebuild-allowlist (after vetting).",
              file=sys.stderr)

    slugs = list_slugs() if args.all else ([args.slug] if args.slug else [])
    if not slugs:
        ap.error("one of --slug, --all, or --rebuild-allowlist is required")

    results = [gate_slug(s, args.base, allow, suppress, args.require_scanners) for s in slugs]
    blocked = [r for r in results if r["verdict"] == "block"]

    if args.json:
        print(json.dumps(results if args.all else results[0], indent=2))
    else:
        for r in results:
            print(f"[{'BLOCK' if r['verdict'] == 'block' else 'pass'}] {r['slug']} ({r['scope']}): "
                  f"{r['p1_count']} P1 / {len(r['findings'])} findings")
            for f in r["findings"]:
                print(f"    {f['severity']} {f['tool']}:{f['rule']} {f['file']}:{f['line']} - {f['evidence']}")
        missing = [t for t, ok in results[0]["tools"].items() if not ok] if results else []
        if missing:
            print(f"\nNOTE: external scanners not installed locally: {', '.join(missing)} "
                  "(CI installs them; built-in deterministic checks still ran).")

    return 1 if blocked else 0


if __name__ == "__main__":
    raise SystemExit(main())
