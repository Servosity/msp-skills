#!/usr/bin/env python3
"""Self-test for replay_handfixes.py (python3 tools/maintainer/replay_handfixes.py --self-test).

Builds a throwaway git repository: a connector as it stood before a reprint is
committed (HEAD), then a press 4.34.0-shaped print replaces its generated files
in the working tree, the way onboard.py vendors one. Every recipe, the
retirement path, keep_manual, delegated ids, the dependency floors and every
fail-closed path are driven both ways. The ledger proof is the REAL
check_handfixes.py (subprocess, as in a run); go build, go vet, go test and the
gates are recorded by a stand-in, because fixtures cannot be compiled. No
network, no Go toolchain needed.
"""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent

REGISTRY = {
    "handfixes": {
        "fx-retired": {"verdict": "retire", "retire_at": "4.34.0", "reason": "the press generates it",
                       "native_asserts": [{"file": "cli/internal/mcp/cobratree/shellout.go",
                                           "contains": "nativeGate(", "code_only": True}],
                       "gates": [{"kind": "check", "argv": ["check_fixture.py", "--slug", "{slug}"]}]},
        "fx-manual": {"verdict": "retire", "retire_at": "4.34.0", "reason": "r",
                      "native_asserts": [{"file": "cli/internal/mcp/cobratree/shellout.go", "contains": "manualGate("}],
                      "keep_manual": {"acme": "this connector's own part"}},
        "fx-delegated": {"verdict": "delegated", "tool": "tools/maintainer/apply_secret_funnel.py"},
        "token-request-stays-on-origin": {"verdict": "replay", "recipe": "token-host-compare"},
        "dry-run-never-mints": {"verdict": "replay", "recipe": "auth-dry-run",
                                "gates": [{"kind": "class", "class": "write-safety"}]},
        "doctor-verdict-tracks-reality": {"verdict": "replay", "recipe": "doctor-verdict"},
        "no-create-alias-on-destructive-commands": {"verdict": "replay", "recipe": "create-alias-removal"},
        "derive-lock-tests-deterministic": {"verdict": "replay", "recipe": "derive-lock-canonical"},
        "no-config-write-switch": {"verdict": "replay", "recipe": "no-config-write"},
        "install-credentials-declared-on-every-channel": {"verdict": "replay", "recipe": "manifest-env-union"},
    },
    "floors": [
        {"module": "golang.org/x/text", "min": "v0.42.0", "applies_from": "4.34.0", "reason": "r"},
        {"module": "github.com/klauspost/compress", "min": "v1.18.7", "applies_from": "4.34.0", "reason": "r"},
    ],
}


def run(m) -> int:  # m: the replay_handfixes module
    ok = True

    def expect(cond: bool, msg: str) -> None:
        nonlocal ok
        print(f"  {'ok  ' if cond else 'FAIL'} {msg}")
        ok = ok and cond

    # ---- pure helpers ------------------------------------------------------
    src = ('package x\n\nimport (\n\t"fmt"\n\t"net/url"\n\t"strings"\n)\n\n'
           '// Doc for A.\nfunc A() string { return fmt.Sprint("{") }\n\n'
           'var m = map[string]bool{\n\t"a": true,\n}\n\n'
           'const C = "x(y"\n\n'
           'func (c *T) M(s string) {\n\t_ = strings.TrimSpace(s)\n}\n\n'
           'var (\n\tg1 = 1\n)\n')
    decls = {d["name"]: d for d in m.go_decls(src)}
    expect(set(decls) == {"A", "m", "C", "T.M", None}, f"go_decls names funcs, methods, vars and consts ({sorted(map(str, decls))})")
    expect(src[decls["A"]["start"]:].startswith("// Doc for A."), "go_decls includes the doc comment above a declaration")
    pruned = m.prune_unused_imports(src[:decls["A"]["start"]] + src[decls["A"]["end"]:], "x.go")
    expect('"fmt"' not in pruned and '"strings"' in pruned and '"net/url"' not in pruned,
           "prune_unused_imports drops only imports no code uses")
    expect('"net/netip"' in m.add_import(src, "net/netip") and m.add_import(src, "fmt") == src,
           "add_import adds a missing import once")
    expect(m.semver("v0.14.0") < m.semver("v0.42.0") < m.semver("v1.0.0") and m.semver("v1.2.3-rc1") < m.semver("v1.2.3"),
           "semver orders versions, a pre-release below its release")
    order = ["v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta", "v1.0.0-beta.2",
             "v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0", "v0.43.0-rc.9"]
    expect(sorted(order[:-1], key=m.semver) == order[:-1] and m.semver("v0.43.0-rc.9") < m.semver("v0.43.0-rc.10"),
           "semver orders pre-release identifiers numerically and by SemVer precedence (rc.9 < rc.10)")
    expect(m.effective_version('{"Version": "v0.42.0", "Replace": {"Path": "golang.org/x/text", "Version": "v0.14.0"}}',
                               "golang.org/x/text") == ("v0.14.0", True)
           and m.effective_version('{"Version": "v0.42.0"}', "golang.org/x/text") == ("v0.42.0", False),
           "a replace directive's version is the one a floor is checked against")
    try:
        m.effective_version('{"Version": "v0.42.0", "Replace": {"Path": "../text"}}', "golang.org/x/text")
        expect(False, "a module replaced by a local directory cannot be verified and is refused")
    except m.GoListError:
        expect(True, "a module replaced by a local directory cannot be verified and is refused")

    # Every Go command the tool starts, directly or through a gate script, runs
    # with workspace mode off: in workspace mode even a read-only go list may
    # write go.work.sum at the workspace root, outside the connector.
    expect(m.go_env().get("GOWORK") == "off" and m.go_env(writable=True).get("GOWORK") == "off"
           and m.gate_env().get("GOWORK") == "off",
           "Go runs with GOWORK=off, here and in every gate script, so no workspace file outside the connector is written")

    # ---- fixture repository ------------------------------------------------
    with tempfile.TemporaryDirectory(prefix="replay-handfixes-") as td:
        root = Path(td)
        tools = root / "tools" / "maintainer"
        tools.mkdir(parents=True)
        shutil.copyfile(HERE / "check_handfixes.py", tools / "check_handfixes.py")
        (tools / "fleet_handfixes.json").write_text(json.dumps(REGISTRY), encoding="utf-8")
        git = lambda *a: subprocess.run(["git", "-C", str(root), *a], capture_output=True, text=True)  # noqa: E731
        git("init", "-q", "-b", "main")
        git("config", "user.email", "t@example.test")
        git("config", "user.name", "t")
        skill = root / "skills" / "acme"
        before(skill, m)
        git("add", "-A")
        git("commit", "-q", "-m", "pre-reprint")
        reprint(skill, m)

        # A stand-in prover: the REAL ledger check, the rest recorded.
        asked: list[dict] = []

        def prove(r, slug, full, p, pkgs):
            asked.append({"full": full, "gates": [h for h, _ in p.gates], "tests": dict(p.tests), "pkgs": pkgs,
                          "floors": list(p.floor_raises)})
            return m.run_proofs(r, slug, False, p, pkgs)

        floors = {"golang.org/x/text": "v0.14.0"}
        raised: list[tuple[str, str]] = []
        real = (m.resolved_version, m.raise_floor)

        def fake_resolved(cli, module):
            return floors.get(module), False

        def fake_raise(cli, module, want):
            raised.append((module, want))
            floors[module] = want
            return want

        m.resolved_version, m.raise_floor = fake_resolved, fake_raise
        sink: list[str] = []
        say = sink.append
        try:
            listed = m.plan_connector(root, "acme", "HEAD", REGISTRY)
            outs = sorted({f"skills/acme/{r}" for r in listed.texts
                           if listed.texts[r] != (listed.orig.get(r) if (skill / r).exists() else None)})
            expect("skills/acme/handfixes.json" in outs and "skills/acme/cli/internal/cli/doctor_verdict.go" in outs,
                   "the plan names the ledger and the restored files (what --list-outputs prints)")

            rc = m.run_slug(root, "acme", check=True, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 1 and any("would change" in s for s in sink)
                   and any("would raise golang.org/x/text v0.14.0 -> v0.42.0" in s for s in sink),
                   "--check on a fresh print reports the replay and the floor, changes nothing, and fails")
            expect(not (skill / "cli/internal/cli/doctor_verdict.go").exists(), "--check wrote nothing")

            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 0 and any("applied and proven" in s for s in sink),
                   "apply on a fresh print re-applies every replay id and passes the real ledger check: "
                   + " | ".join(s for s in sink if "NEEDS" in s or "PROOF" in s or "FAIL" in s)[:600])
            led = json.loads((skill / "handfixes.json").read_text())
            ids = [e["id"] for e in led["handfixes"]]
            expect("fx-retired" not in ids and any(r.get("id") == "fx-retired" for r in led.get("retired", [])),
                   "a retired id leaves the ledger and is kept as a retired record with the registry's reason")
            expect("fx-manual" in ids and "fx-delegated" in ids, "keep_manual and delegated entries are left alone")
            expect(not (skill / "cli/internal/mcp/cobratree/handfix_gate_test.go").exists(),
                   "a retired entry's own hand-written file, carried in by the reprint, is removed")
            last = asked[-1] if asked else {"full": False, "gates": [], "tests": {}}
            expect(bool(last["full"]) and "fx-retired" in last["gates"] and "dry-run-never-mints" in last["gates"],
                   "an apply runs the retired and replayed ids' gates")
            expect("internal/cli" in last["tests"] and "TestAuthLoginDryRunSendsNothing" in last["tests"]["internal/cli"],
                   "an apply runs the tests of every file it restored")
            expect(raised == [("golang.org/x/text", "v0.42.0")], "the x/text floor is raised; compress is not in the graph")
            cli = skill / "cli"
            oauth = (cli / "internal/cliutil/oauth_token.go").read_text()
            expect("if !oauthTokenSameHost(target.Hostname(), origin.Hostname()) {" in oauth
                   and "func oauthTokenSameHost(a, b string) bool {" in oauth and '"net/netip"' in oauth,
                   "token-host-compare: the compare, the helper and its import")
            auth = (cli / "internal/cli/auth.go").read_text()
            expect(all(x in auth for x in ("(dry run - no request sent, nothing saved)", "(dry run - nothing saved)",
                                           "(dry run - nothing changed)", "if config.NoConfigWrite() {")),
                   "auth-dry-run and no-config-write: every command wired")
            t = (cli / "internal/cli/auth_dry_run_test.go").read_text()
            expect('run("stub-dry-run-token\\n", "auth", "set-token", "--dry-run")' in t,
                   "a restored set-token test is moved to stdin when the print reads the token from stdin")
            doc = (cli / "internal/cli/doctor.go").read_text()
            expect('doctorProbeCredentials(cmd.Context(), c, cmd.Root(), "acme-cli", report)' in doc
                   and "+ doctorBaseURLEnv +" in doc and "nothing was probed" in doc,
                   "doctor-verdict: probe, absent credential and base-URL variable wired")
            helper = (cli / "internal/cli/doctor_verdict.go").read_text()
            expect("func doctorIsInfoKey(" not in helper and '"sort"' not in helper and "func doctorProbeCredentials(" in helper,
                   "the restored helper drops a declaration the print now generates, and its now-unused import")
            expect("func doctorBaseURLIsShippedPlaceholder(" in helper and "doctorBaseURLIsShippedPlaceholder(cfg.BaseURL)" in doc,
                   "a hand-fix stronger than the press's copy is kept under its own name and still called")
            de = next(e for e in led["handfixes"] if e["id"] == "doctor-verdict-tracks-reality")
            expect(any(a.get("file") == "cli/internal/cli/doctor.go" and a.get("contains") == "func doctorBaseURLIsPlaceholder("
                       for a in de["asserts"]), "the assert on the dropped declaration now pins the generated file")
            dr = next(e for e in led["handfixes"] if e["id"] == "dry-run-never-mints")
            expect(not any("needsClientCredentialsMint" in (a.get("contains") or "") for a in dr["asserts"]),
                   "an assert the print makes obsolete is dropped with its reason")
            expect(any(a.get("contains") == "(dry run - nothing changed)" and a.get("code_only") is True
                       and a.get("file") == "cli/internal/cli/auth.go" for a in dr["asserts"]),
                   "an edit the replay made that the entry did not pin (logout) is pinned in the ledger")
            th = next(e for e in led["handfixes"] if e["id"] == "token-request-stays-on-origin")
            expect(any(a.get("contains") == "if !oauthTokenSameHost(target.Hostname(), origin.Hostname()) {"
                       for a in th["asserts"]), "the replayed call site is pinned in the ledger")
            expect('[]string{"create"}' not in (cli / "internal/cli/things_item_purge_purge.go").read_text(),
                   "create-alias-removal: the destructive command loses its alias, found by Use and Short after a rename")
            ca = next(e for e in led["handfixes"] if e["id"] == "no-create-alias-on-destructive-commands")
            expect(any(a.get("file") == "cli/internal/cli/things_item_purge_purge.go" and "not_contains" in a
                       for a in ca["asserts"]) and not any(a.get("file") == "cli/internal/cli/purge.go" for a in ca["asserts"]),
                   "its assert moves to the renamed file")
            expect('{"things", "item-purge", "purge"}' in (cli / "internal/cli/create_alias_destructive_test.go").read_text(),
                   "the restored test follows the command to its new path, read from the code (Use and AddCommand)")
            expect((cli / "internal/learn/derive_test.go").read_text() == m.render("derive_test.go.tmpl", MODULE="acme-pp-cli"),
                   "derive-lock-canonical: the known generated test is replaced by the canonical one")
            cfg = (cli / "internal/config/config.go").read_text()
            expect("func NoConfigWrite() bool {" in cfg and "return c.saveCredentialMaterial()" in cfg
                   and "\treturn c.saveCredentialsThenConfig()\n}\n\nfunc (c *Config) ClearTokens" not in cfg,
                   "no-config-write: the switch, and SaveTokens routed through it")
            man = json.loads((skill / "manifest.json").read_text())
            expect(set(man["server"]["mcp_config"]["env"]) == {"ACME_CLIENT_ID", "ACME_CLIENT_SECRET", "ACME_BASE_URL",
                                                               "ACME_NO_CONFIG_WRITE"}
                   and "acme_base_url" in man["user_config"],
                   "manifest-env-union: every pre-reprint env key and its prompt is declared again")

            snap = {p: p.read_bytes() for p in skill.rglob("*") if p.is_file()}
            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 0 and snap == {p: p.read_bytes() for p in skill.rglob("*") if p.is_file()}
                   and any("already applied" in s for s in sink), "a re-apply is a no-op that proves again")
            sink.clear()
            expect(m.run_slug(root, "acme", check=True, out=say, prove=prove, registry=REGISTRY) == 0,
                   "--check on the replayed tree passes")

            # The ledger check is real: removing a replayed marker fails the proof.
            (cli / "internal/cliutil/oauth_token.go").write_text(oauth.replace("oauthTokenSameHost(target", "strings.EqualFold(target"))
            sink.clear()
            rc = m.run_slug(root, "acme", check=True, out=say, prove=lambda *a: m.run_proofs(a[0], a[1], False, a[3], a[4]),
                            registry=REGISTRY)
            expect(rc == 1, "a replayed marker removed by hand is found again (would re-apply)")
            (cli / "internal/cliutil/oauth_token.go").write_text(oauth)
            native = cli / "internal/mcp/cobratree/shellout.go"
            native_text = native.read_text()
            native.write_text(native_text.replace("nativeGate(", "otherGate("))
            sink.clear()
            rc = m.run_slug(root, "acme", check=True, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 1 and any("PROOF FAILED" in s and "'fx-retired (retired, native)' REGRESSED" in s for s in sink),
                   "a retirement whose native shape is gone fails the proof (a retirement is never a silent drop)")
            native.write_text(native_text)

            # Fail closed: each site the recipes cannot anchor is named.
            git("stash", "-q", "--include-untracked")
            reprint(skill, m, break_sites=True)
            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            named = " ".join(s for s in sink if "NEEDS HAND WIRING" in s)
            expect(rc == 1, "a print the recipes cannot anchor fails")
            for site in ("[dry-run:newAuthLoginCmd]", "[token-host]", "[derive-test]", "[doctor-variant]",
                         "[ncw-save]", "[restore:cli/internal/cli/never_test.go]"):
                expect(site in named, f"fail-closed: {site} is named")
            git("checkout", "-q", "--", ".")
            git("clean", "-fdq")
            git("stash", "pop", "-q")

            # Below the retire version (main today): nothing retires, a replay
            # id whose hand-fix is present is already applied, floors only note.
            git("stash", "-q", "--include-untracked")
            floors.update({"golang.org/x/text": "v0.39.0"})
            raised.clear()
            sink.clear()
            rc = m.run_slug(root, "acme", check=True, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 0 and any("already applied" in s for s in sink) and not raised,
                   "on a pre-4.34.0 connector --check reports already applied: " + " | ".join(sink)[:400])
            expect(any("below the v0.42.0 floor" in s for s in sink), "a pre-4.34.0 connector below a floor gets a note only")
            git("stash", "pop", "-q")

            # Containment: a ledger path that leaves the connector is never read or written.
            other = root / "skills" / "other" / "README.md"
            w(other, '\tAliases: []string{"create"},\n')
            led2 = json.loads((skill / "handfixes.json").read_text())
            for e in led2["handfixes"]:
                if e["id"] == "no-create-alias-on-destructive-commands":
                    e["asserts"].append({"file": "cli/../../other/README.md", "not_contains": '[]string{"create"}'})
            saved_ledger = (skill / "handfixes.json").read_text()
            (skill / "handfixes.json").write_text(json.dumps(led2, indent=1) + "\n")
            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 1 and other.read_text() == '\tAliases: []string{"create"},\n'
                   and any("[path]" in s for s in sink), "a ledger path outside the connector is refused and never written")
            (skill / "handfixes.json").write_text(saved_ledger)
            expect(m.run_slug(root, "../acme", check=True, out=say, prove=prove, registry=REGISTRY) == 1,
                   "a slug that is not a plain name is refused")

            # Containment covers subprocess writes too: go get and go mod tidy
            # write cli/go.mod and cli/go.sum, so a symlinked one is refused
            # before any Go command runs, and planning does not crash on it.
            outside = root / "skills" / "other" / "go.sum"
            w(outside, "outside\n")
            gosum = skill / "cli" / "go.sum"
            had = gosum.read_text() if gosum.exists() else None
            if gosum.exists():
                gosum.unlink()
            gosum.symlink_to(Path("..") / ".." / "other" / "go.sum")
            floors["golang.org/x/text"] = "v0.14.0"
            raised.clear()
            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 1 and not raised and outside.read_text() == "outside\n" and any("[path]" in s for s in sink),
                   "a symlinked cli/go.sum is refused before go get or go mod tidy could write through it")
            gosum.unlink()
            if had is not None:
                gosum.write_text(had)
            gomod = skill / "cli" / "go.mod"
            real_mod = gomod.read_text()
            gomod.unlink()
            w(root / "skills" / "other" / "go.mod", "module other\n")
            gomod.symlink_to(Path("..") / ".." / "other" / "go.mod")
            sink.clear()
            try:
                rc = m.run_slug(root, "acme", check=True, out=say, prove=prove, registry=REGISTRY)
                expect(rc == 1 and any("[path]" in s for s in sink), "a symlinked cli/go.mod is named, not followed")
            except AttributeError as e:
                expect(False, f"a symlinked cli/go.mod is named, not followed (crashed: {e})")
            gomod.unlink()
            gomod.write_text(real_mod)
            floors["golang.org/x/text"] = "v0.42.0"

            # Floors: a replace directive below the floor is named, never trusted.
            saved = m.resolved_version
            m.resolved_version = lambda cli, module: ("v0.14.0", True) if module == "golang.org/x/text" else (None, False)
            floors["golang.org/x/text"] = "v0.14.0"
            p = m.plan_connector(root, "acme", "HEAD", REGISTRY)
            expect(any(site == "floor:golang.org/x/text" and "replace" in msg for site, msg in p.needs_hand)
                   and not p.floor_raises, "a module a replace directive pins below the floor is named, not raised")
            m.resolved_version = saved

            # Floors: met, retired, pinned higher at base, and a raise that fails.
            floors["golang.org/x/text"] = "v0.42.0"
            p = m.plan_connector(root, "acme", "HEAD", REGISTRY)
            expect(not p.floor_raises, "a floor the print already meets is a no-op")
            reg2 = json.loads(json.dumps(REGISTRY))
            reg2["floors"][0]["retire_at"] = "4.34.0"
            floors["golang.org/x/text"] = "v0.14.0"
            expect(not m.plan_connector(root, "acme", "HEAD", reg2).floor_raises, "a floor past its retire_at is skipped")
            reg3 = json.loads(json.dumps(REGISTRY))
            reg3["floors"][0]["min"] = "v0.20.0"
            p = m.plan_connector(root, "acme", "HEAD", reg3)
            expect(p.floor_raises and p.floor_raises[0][2] == "v0.41.0",
                   "a floor never lowers what the pre-reprint go.mod pinned (want = v0.41.0 from HEAD)")
            m.raise_floor = lambda cli, module, want: None
            sink.clear()
            rc = m.run_slug(root, "acme", check=False, out=say, prove=prove, registry=REGISTRY)
            expect(rc == 1 and any("[floor:golang.org/x/text]" in s for s in sink), "a floor that cannot be raised is named")
        finally:
            m.resolved_version, m.raise_floor = real

    print("PASS: replay_handfixes self-test" if ok else "FAIL: replay_handfixes self-test")
    return 0 if ok else 1


# --------------------------------------------------------------------------- #
# Fixture connector.
# --------------------------------------------------------------------------- #

def w(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


GEN = "// Code generated by CLI Printing Press. DO NOT EDIT.\n\n"
HELPER = """package cli

import (
	"context"
	"net/url"
	"sort"
)

// doctorIsInfoKey is the old hand-fix copy; the press generates it now.
func doctorIsInfoKey(key string) bool { return sort.SearchStrings(nil, key) == 0 }

const doctorBaseURLEnv = "ACME_BASE_URL"

// doctorBaseURLIsPlaceholder is the old hand-fix copy.
func doctorBaseURLIsPlaceholder(base string) bool {
	_, err := url.Parse(base)
	return err != nil
}

func doctorProbeCredentials(ctx context.Context, c *client.Client, root *cobra.Command, bin string, report map[string]any) {
	_ = ctx
}

func doctorUnexpectedStatus(status int, path string, report map[string]any) {
	_ = status
}
"""
SWITCH = """// NoConfigWriteEnv is the environment switch (issue #270).
const NoConfigWriteEnv = "ACME_NO_CONFIG_WRITE"

// NoConfigWrite reports whether the switch is on.
func NoConfigWrite() bool {
	return os.Getenv(NoConfigWriteEnv) != ""
}

// saveCredentialMaterial is the single entry point.
func (c *Config) saveCredentialMaterial() error {
	if NoConfigWrite() {
		return nil
	}
	return c.save()
}
"""


def ledger(m) -> dict:
    A = lambda f, n, **k: dict({"file": f, "contains": n}, **k)  # noqa: E731
    N = lambda f, n: {"file": f, "not_contains": n}  # noqa: E731
    return {"slug": "acme", "doc": "docs/reprint-survival.md", "handfixes": [
        {"id": "fx-retired", "asserts": [A("cli/internal/mcp/cobratree/shellout.go", "handFixGate("),
                                         A("cli/internal/mcp/cobratree/handfix_gate_test.go", "func TestHandFixGate(")]},
        {"id": "fx-manual", "asserts": [A("cli/internal/mcp/cobratree/shellout.go", "manualGate(")]},
        {"id": "fx-delegated", "asserts": [A("cli/internal/client/client.go", "funnelThing(")]},
        {"id": "token-request-stays-on-origin", "asserts": [
            A("cli/internal/cliutil/oauth_token.go", "func oauthTokenSameHost(a, b string) bool {", code_only=True),
            N("cli/internal/cliutil/oauth_token.go", "if !strings.EqualFold(target.Hostname(), origin.Hostname()) {"),
            A("cli/internal/cliutil/oauth_token_origin_test.go", "func TestOAuthTokenSameOriginComparesHostsLikeTheNetwork(")]},
        {"id": "dry-run-never-mints", "asserts": [
            A("cli/internal/client/client.go", "if !c.DryRun && needsClientCredentialsMint(c.Config) {"),
            A("cli/internal/cli/auth.go", "(dry run - no request sent, nothing saved)"),
            A("cli/internal/cli/auth.go", "(dry run - nothing saved)"),
            A("cli/internal/cli/auth_dry_run_test.go", "func TestAuthLoginDryRunSendsNothing(")]},
        {"id": "doctor-verdict-tracks-reality", "asserts": [
            A("cli/internal/cli/doctor_verdict.go", "func doctorProbeCredentials("),
            A("cli/internal/cli/doctor_verdict.go", "func doctorBaseURLIsPlaceholder("),
            A("cli/internal/cli/doctor.go", "doctorProbeCredentials(cmd.Context()"),
            N("cli/internal/cli/doctor.go", "present, not verified. Run `%s %s`")]},
        {"id": "no-create-alias-on-destructive-commands", "asserts": [
            N("cli/internal/cli/purge.go", '[]string{"create"}'),
            A("cli/internal/cli/create_alias_destructive_test.go", "func TestDestructiveCommandsDoNotAnswerToCreate(")]},
        {"id": "derive-lock-tests-deterministic", "asserts": [
            A("cli/internal/learn/derive_test.go", "func deriveBlockedOnLock() bool {", code_only=True)]},
        {"id": "no-config-write-switch", "asserts": [
            A("cli/internal/config/config.go", 'const NoConfigWriteEnv = "ACME_NO_CONFIG_WRITE"'),
            A("cli/internal/config/config.go", "return c.saveCredentialMaterial()"),
            A("cli/internal/cli/auth.go", "if config.NoConfigWrite() {"),
            A("cli/internal/config/config_noconfigwrite_test.go", "func TestNoConfigWriteLeavesNothingOnDisk("),
            A("cli/internal/mcp/mcp_config_resolution_test.go", "func TestMCPClientHonoursTheConfigEnvVar("),
            A("manifest.json", '"ACME_NO_CONFIG_WRITE"')]},
        {"id": "install-credentials-declared-on-every-channel", "asserts": [
            A("manifest.json", '"ACME_BASE_URL": "${user_config.acme_base_url}"')]},
    ]}


def before(skill: Path, m) -> None:
    """The connector as committed before the reprint (press 4.33.5, hand-fixed)."""
    cli = skill / "cli"
    w(cli / "go.mod", "module acme-pp-cli\n\ngo 1.26.10\n\nrequire golang.org/x/text v0.41.0 // indirect\n")
    w(skill / "manifest.json", json.dumps({"printing_press_version": "4.33.5",
                                           "press_provenance": {"printing_press_version": "4.33.5"}, "server": {"mcp_config": {"env": {
        "ACME_CLIENT_ID": "${user_config.acme_client_id}", "ACME_CLIENT_SECRET": "${user_config.acme_client_secret}",
        "ACME_BASE_URL": "${user_config.acme_base_url}", "ACME_NO_CONFIG_WRITE": "${user_config.acme_no_config_write}"}}},
        "user_config": {k: {"type": "string"} for k in ("acme_client_id", "acme_client_secret", "acme_base_url",
                                                         "acme_no_config_write")}}, indent=2) + "\n")
    w(skill / "press/.printing-press.json", json.dumps({"printing_press_version": "4.33.5"}) + "\n")
    w(cli / "internal/mcp/cobratree/shellout.go", GEN + "package cobratree\n\nfunc handFixGate() {}\nfunc nativeGate() {}\nfunc manualGate() {}\n")
    w(cli / "internal/mcp/cobratree/handfix_gate_test.go", "package cobratree\n\nfunc TestHandFixGate(t *testing.T) {}\n")
    w(cli / "internal/client/client.go", GEN + "package client\n\nfunc funnelThing() {}\n"
      "func x() {\n\tif !c.DryRun && needsClientCredentialsMint(c.Config) {\n\t}\n}\n")
    w(cli / "internal/cliutil/oauth_token.go", GEN + 'package cliutil\n\nimport (\n\t"strings"\n)\n\n'
      "func oauthTokenSameOrigin(target, origin *url.URL) bool {\n" + m.OAUTH_HOST + "\t\treturn false\n\t}\n\treturn true\n}\n"
      + m.OAUTH_HELPER)
    w(cli / "internal/cliutil/oauth_token_origin_test.go",
      "package cliutil\n\nfunc TestOAuthTokenSameOriginComparesHostsLikeTheNetwork(t *testing.T) {}\n")
    w(cli / "internal/cli/auth.go", GEN + "package cli\n\n// (dry run - no request sent, nothing saved) (dry run - nothing saved)\n"
      "func f() {\n\tif config.NoConfigWrite() {\n\t}\n}\n")
    w(cli / "internal/cli/auth_dry_run_test.go", "package cli_test\n\nfunc TestAuthLoginDryRunSendsNothing(t *testing.T) {\n"
      '\trun("", "auth", "set-token", "stub-dry-run-token", "--dry-run")\n}\n')
    w(cli / "internal/cli/doctor_verdict.go", HELPER)
    w(cli / "internal/cli/doctor.go", GEN + "package cli\n\nfunc d() {\n\tdoctorProbeCredentials(cmd.Context(), c, nil, \"\", nil)\n}\n")
    w(cli / "internal/cli/purge.go", GEN + "package cli\n\nfunc newPurgeCmd(flags *rootFlags) *cobra.Command {\n"
      "\treturn &cobra.Command{\n\t\tUse:   \"purge <id>\",\n\t\tShort: \"Purge things\",\n\t}\n}\n")
    w(cli / "internal/cli/create_alias_destructive_test.go",
      "package cli\n\nfunc TestDestructiveCommandsDoNotAnswerToCreate(t *testing.T) {\n\t_ = [][]string{\n\t\t{\"purge\"},\n\t}\n}\n")
    w(cli / "internal/cli/gone_test.go", "package cli\n\nfunc TestGone(t *testing.T) {}\n")
    w(cli / "internal/learn/derive_test.go", m.render("derive_test.go.tmpl", MODULE="acme-pp-cli"))
    w(cli / "internal/config/config.go", GEN + 'package config\n\nimport (\n\t"os"\n)\n\n' + SWITCH
      + "\nfunc (c *Config) SaveTokens() error {\n\treturn c.saveCredentialMaterial()\n}\n")
    w(cli / "internal/config/config_noconfigwrite_test.go",
      "package config\n\nfunc TestNoConfigWriteLeavesNothingOnDisk(t *testing.T) {}\n")
    w(cli / "internal/mcp/mcp_config_resolution_test.go",
      "package mcp\n\nfunc TestMCPClientHonoursTheConfigEnvVar(t *testing.T) {\n\tc, err := newMCPClient()\n}\n")
    w(cli / "internal/mcp/tools.go", GEN + "package mcp\n")
    w(skill / "handfixes.json", json.dumps(ledger(m), indent=1) + "\n")


def reprint(skill: Path, m, break_sites: bool = False) -> None:
    """Replace the generated files the way onboard vendors a press 4.34.0 print:
    hand-written files are gone, generated ones take the 4.34.0 shapes, the
    manifest is the press's (credential pair only), the version is stamped."""
    cli = skill / "cli"
    for rel in ("internal/cliutil/oauth_token_origin_test.go", "internal/cli/auth_dry_run_test.go",
                "internal/cli/doctor_verdict.go", "internal/cli/create_alias_destructive_test.go",
                "internal/cli/gone_test.go", "internal/config/config_noconfigwrite_test.go",
                "internal/mcp/mcp_config_resolution_test.go"):
        (cli / rel).unlink(missing_ok=True)
    w(cli / "go.mod", "module acme-pp-cli\n\ngo 1.26.10\n")
    man = json.loads((skill / "manifest.json").read_text())
    man["printing_press_version"] = "4.34.0"
    man["press_provenance"] = {"printing_press_version": "4.34.0"}
    man["server"]["mcp_config"]["env"] = {"ACME_CLIENT_ID": "${user_config.acme_client_id}",
                                          "ACME_CLIENT_SECRET": "${user_config.acme_client_secret}"}
    man["user_config"] = {"acme_client_id": {"type": "string"}, "acme_client_secret": {"type": "string"}}
    w(skill / "manifest.json", json.dumps(man, indent=2) + "\n")
    w(skill / "press/.printing-press.json", json.dumps({"printing_press_version": "4.34.0"}) + "\n")
    # fx-retired's own hand-written test was carried in by the reprint.
    w(cli / "internal/mcp/cobratree/shellout.go", GEN + "package cobratree\n\nfunc nativeGate() {}\nfunc manualGate() {}\n")
    w(cli / "internal/client/client.go", GEN + "package client\n\nfunc funnelThing() {}\n")
    fold = m.OAUTH_FOLD + ("\t\treturn false\n\t}\n" + m.OAUTH_FOLD if break_sites else "")
    w(cli / "internal/cliutil/oauth_token.go", GEN + 'package cliutil\n\nimport (\n\t"strings"\n)\n\n'
      "func oauthTokenSameOrigin(target, origin *url.URL) bool {\n" + fold + "\t\treturn false\n\t}\n\treturn true\n}\n")
    login = "" if break_sites else m.DRY_LOGIN[0]
    w(cli / "internal/cli/auth.go", GEN + "package cli\n\n"
      "func newAuthLoginCmd(flags *rootFlags) *cobra.Command {\n\tcmd := &cobra.Command{\n" + m.RUNE
      + "\t\t\ttokenURL := cfg.TokenURL\n" + login + "\t\t\treturn nil\n\t\t},\n\t}\n\treturn cmd\n}\n\n"
      "func newAuthSetTokenCmd(flags *rootFlags) *cobra.Command {\n\treturn &cobra.Command{\n" + m.RUNE
      + "\t\t\ttoken, err := readSecretFromStdin(cmd.InOrStdin())\n" + m.DRY_SET[0] + "\t\t\t}\n\t\t\treturn nil\n\t\t},\n\t}\n}\n\n"
      "func newAuthLogoutCmd(flags *rootFlags) *cobra.Command {\n\treturn &cobra.Command{\n" + m.RUNE
      + m.DRY_LOGOUT[0] + "\t\t\t}\n\t\t\treturn nil\n\t\t},\n\t}\n}\n")
    suggest = ('\t\t\t\t\t\tsuggestion := suggestReadCommand(cmd.Root())\n\t\t\t\t\t\tif suggestion != "" {\n'
               '\t\t\t\t\t\t\treport["credentials"] = fmt.Sprintf("present, not verified. Run `%s %s` to confirm the token works end-to-end.", "acme-cli", suggestion)\n'
               '\t\t\t\t\t\t} else {\n'
               '\t\t\t\t\t\t\treport["credentials"] = "present, not verified. Run any read command to confirm the token works end-to-end."\n'
               '\t\t\t\t\t\t}\n')
    w(cli / "internal/cli/doctor.go", GEN + 'package cli\n\nimport (\n\t"strings"\n)\n\n'
      "func doctorIsInfoKey(key string) bool { return key == \"\" }\n\n"
      "func newDoctorCmd(flags *rootFlags) *cobra.Command {\n"
      "\t\t\tif cfg != nil && doctorBaseURLIsPlaceholder(cfg.BaseURL) {\n\t\t\t\t" + m.DOCTOR_PLACEHOLDER
      + "\t\t\t}\n\t\t\t\t\t\t" + m.DOCTOR_NOAUTH + suggest + "\treturn nil\n}\n\n"
      "func doctorBaseURLIsPlaceholder(base string) bool {\n\treturn strings.TrimSpace(base) == \"\"\n}\n")
    (cli / "internal/cli/purge.go").unlink(missing_ok=True)
    w(cli / "internal/cli/root.go", GEN + "package cli\n\nfunc newRootCmd(flags *rootFlags) *cobra.Command {\n"
      "\tcmd.AddCommand(newThingsCmd(flags))\n\treturn cmd\n}\n")
    w(cli / "internal/cli/things.go", GEN + "package cli\n\nfunc newThingsCmd(flags *rootFlags) *cobra.Command {\n"
      "\tcmd := &cobra.Command{\n\t\tUse:   \"things\",\n\t}\n\tcmd.AddCommand(newThingsItemPurgeCmd(flags))\n\treturn cmd\n}\n")
    w(cli / "internal/cli/things_item_purge.go", GEN + "package cli\n\nfunc newThingsItemPurgeCmd(flags *rootFlags) *cobra.Command {\n"
      "\tcmd := &cobra.Command{\n\t\tUse:   \"item-purge\",\n\t}\n\tcmd.AddCommand(newThingsItemPurgePurgeCmd(flags))\n\treturn cmd\n}\n")
    w(cli / "internal/cli/things_item_purge_purge.go", GEN + "package cli\n\nfunc newThingsItemPurgePurgeCmd(flags *rootFlags) *cobra.Command {\n"
      '\treturn &cobra.Command{\n\t\tUse:   "purge <id>",\n\t\tShort: "Purge things",\n'
      '\t\tAliases:     []string{"create"},\n\t}\n}\n')
    derive = "package learn\n\n// generated derive test\n" + ("// unknown shape\n" if break_sites else "")
    w(cli / "internal/learn/derive_test.go", derive)
    m.DERIVE_GENERATED.add(__import__("hashlib").sha256(("package learn\n\n// generated derive test\n").encode()).hexdigest())
    save = "\treturn c.saveCredentialsThenConfig()\n}\n" if not break_sites else "\treturn c.persist()\n}\n"
    w(cli / "internal/config/config.go", GEN + 'package config\n\nimport (\n\t"os"\n)\n\n'
      "func (c *Config) SaveTokens(clientID string) error {\n\tc.updateFileConfigField(\"TokenExpiry\")\n" + save
      + "\nfunc (c *Config) ClearTokens() error {\n\treturn c.save()\n}\n")
    w(cli / "internal/mcp/tools.go", GEN + 'package mcp\n\nfunc newMCPConfig() (*config.Config, error) {\n\tcfg, err := config.Load("")\n'
      "\treturn cfg, err\n}\n")
    if break_sites:
        w(cli / "internal/cli/doctor_verdict.go", "package cli\n\n// a connector's own variant\nfunc doctorProbeCredentials(x int) {}\n")
        led = json.loads((skill / "handfixes.json").read_text())
        for e in led["handfixes"]:
            if e["id"] == "no-create-alias-on-destructive-commands":
                e["asserts"].append({"file": "cli/internal/cli/never_test.go", "contains": "func TestNever("})
        w(skill / "handfixes.json", json.dumps(led, indent=1) + "\n")
