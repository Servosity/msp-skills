#!/usr/bin/env python3
"""apply_secret_funnel.py - wire the shared secret-echo funnel into a connector.

    python3 tools/maintainer/apply_secret_funnel.py --slug cloudally           # apply
    python3 tools/maintainer/apply_secret_funnel.py --slug cloudally --check   # report, change nothing
    python3 tools/maintainer/apply_secret_funnel.py --check-drift              # ci_guards guard 10
    python3 tools/maintainer/apply_secret_funnel.py --self-test

Why this exists
---------------
The class test `class_harness.py --class secret-echo` fails any connector that
prints a credential a server echoed back in an encoded form (HTML entities,
JSON escapes, double URL-encoding, base64, hex). Before press 4.34.0 the
generated masks cover only the raw and URL-escaped forms (cli-printing-press
#5145), so every fresh print fails it. PR #444 closed the class on 72 connectors
with one shared funnel; this tool makes the same change on a fresh or reprinted
connector, so an onboard needs no hand work for the generated paths.

Press 4.34.0 (#5169) masks encoded forms itself, and its raw output passes the
class test. The funnel stays: a reprint keeps the connector's hand-written code
(token endpoints, novel commands), which never goes through the generated masks
and which the raw class test never exercises. The CLI and MCP error backstops
and the held set are what cover that code.

What it does (every step is idempotent)
---------------------------------------
1. Copies the canonical funnel sources from tools/maintainer/secret_funnel/
   (the ONE copy of record; every connector carries them byte-identical, the
   per-connector files with only the Go module path substituted):
     internal/cliutil/secret_funnel.go (+ _test.go)   the decoder and held set
     internal/cli/secret_funnel_cli.go (+ _test.go)   the CLI error backstop
     cmd/<slug>-mcp/secret_funnel_mcp.go (+ _test.go) the MCP error backstop
2. Makes the anchored edits the generated code needs, for press 4.19 to 4.34.2:
   client.go (maskCredentialText ends in the funnel; the error body is checked
   whole before the display cut; the 2xx HTML summary and the token mint and
   refresh error bodies are funnelled; New holds the client's credentials; the
   resolved auth header is held), root.go (command errors pass the funnel before
   Cobra prints them), every CLI error envelope, the MCP server's outermost tool
   middleware, and the generated `auth login` token exchange. Press 4.34.0
   changed three anchors: maskCredentialText declares `secrets` and ends in
   finishCredentialMask (the funnel wraps that call), and New returns early
   when its HTTP client could not be built (the hold goes before that guard,
   so it covers both returns). Any other New with more than one return is
   still refused.
3. Writes the connector's handfixes.json entry "secret-echo-decoded-funnel"
   (class "secret-echo") with an assert for every edit present, merging into an
   existing entry. One kind of recorded assert is replaced, never kept: an
   assert a reprint made stale, which is one that no longer holds for a site
   whose wiring in the current press shape does hold (SITE_SHAPES: the
   mask-return and error-body sites). Every other recorded assert is kept, so
   a hand-added one that fails still fails the proof.
4. Proves the result by execution, never by reading its own edits back: the
   ledger always passes check_handfixes.py --slug; every apply (one that
   changes nothing too) must also pass go build, go vet and class_harness.py
   --slug --class secret-echo with 0 failing, run against this tree (the
   registry must point the class test at skills/<slug>) and actually exercised
   (an advisory "not exercised" is not a pass unless the entry declares
   "hand_wired": {"class-test": "<how the error path was proven>"}). A failed
   proof exits 1 and names the proof and the sites the run touched. --check
   stays cheap: applied state and the ledger checker, no build.

It fails closed. A required site it cannot anchor, an optional site whose
anchor is ambiguous, and a hand-written token exchange that formats server
text into an error without a funnel call are all listed by file and site, and
the run exits 1, so a human wires that site instead of the tool guessing. It
never reports success for a connector that is not fully wired. It does not
know a credential the connector reads from the environment outside its Config
(a few hand-written auth flows do); the class test catches those.

Exit codes: 0 fully wired and proven (applied, or already applied). 1 a site
needs hand wiring, a proof failed, --check found work to do, or --check-drift
found a drifted copy or an unreadable ledger. 2 bad usage.
"""

from __future__ import annotations

import argparse
import difflib
import json
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from check_handfixes import strip_comments  # noqa: E402  the ledger's own code-only semantics

ROOT = Path(__file__).resolve().parent.parent.parent
CANON = Path(__file__).resolve().parent / "secret_funnel"
ENTRY_ID = "secret-echo-decoded-funnel"
CLASS = "secret-echo"
TAG = "Hand-fix (handfixes.json secret-echo-decoded-funnel)"

# (canonical template, destination under cli/; "MCP" is the MCP command dir)
FILES = (
    ("cliutil_secret_funnel.go.tmpl", "internal/cliutil/secret_funnel.go"),
    ("cliutil_secret_funnel_test.go.tmpl", "internal/cliutil/secret_funnel_test.go"),
    ("cli_secret_funnel.go.tmpl", "internal/cli/secret_funnel_cli.go"),
    ("cli_secret_funnel_test.go.tmpl", "internal/cli/secret_funnel_cli_test.go"),
    ("mcp_secret_funnel.go.tmpl", "MCP/secret_funnel_mcp.go"),
    ("mcp_secret_funnel_test.go.tmpl", "MCP/secret_funnel_mcp_test.go"),
)

# --------------------------------------------------------------------------- #
# The anchored edits. Each is (before, after): `after` present means applied.
# --------------------------------------------------------------------------- #

HELD_BEFORE = """	var masks []credentialMask
	seen := map[string]struct{}{}
	addValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
"""
HELD_AFTER = """	var masks []credentialMask
	// Hand-fix (handfixes.json secret-echo-decoded-funnel): every value the
	// masks below cover is also held for the decoded check at the end.
	var held []string
	seen := map[string]struct{}{}
	addValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		held = append(held, value)
"""
RETURN_BEFORE = """	masked := text
	for _, mask := range masks {
		masked = strings.ReplaceAll(masked, mask.needle, mask.replacement)
	}
	return masked
}
"""
RETURN_AFTER = """	masked := text
	for _, mask := range masks {
		masked = strings.ReplaceAll(masked, mask.needle, mask.replacement)
	}
	// Hand-fix (handfixes.json secret-echo-decoded-funnel): the masks cover a
	// value as written and URL-escaped. If the text still carries a held value
	// in any other encoding (HTML entities, JSON escapes, double URL-encoding,
	// base64, hex, split by a control character or a tag), it is withheld whole.
	return cliutil.FunnelWithhold(masked, held)
}
"""
RETURN_MARK = "return cliutil.FunnelWithhold(masked, held)"
# Press 4.34.0 (#5169): maskCredentialText also collects `secrets` for its own
# encoded masks and ends in finishCredentialMask. The funnel wraps that call, so
# its held set and decoder still run over whatever the press's masks leave.
MASKS_LINE = "\tvar masks []credentialMask\n"
SECRETS_LINE = "\tvar secrets []string\n"
HELD_BEFORE_434 = HELD_BEFORE.replace(MASKS_LINE, MASKS_LINE + SECRETS_LINE, 1)
HELD_AFTER_434 = HELD_AFTER.replace(MASKS_LINE, MASKS_LINE + SECRETS_LINE, 1)
RETURN_BEFORE_434 = """	masked := text
	for _, mask := range masks {
		masked = strings.ReplaceAll(masked, mask.needle, mask.replacement)
	}
	return finishCredentialMask(masked, secrets)
}
"""
RETURN_AFTER_434 = """	masked := text
	for _, mask := range masks {
		masked = strings.ReplaceAll(masked, mask.needle, mask.replacement)
	}
	// Hand-fix (handfixes.json secret-echo-decoded-funnel): the press's own
	// masks run first. If the text still carries a held value in any encoding
	// the decoder recognizes, it is withheld whole.
	return cliutil.FunnelWithhold(finishCredentialMask(masked, secrets), held)
}
"""
RETURN_MARK_434 = "return cliutil.FunnelWithhold(finishCredentialMask(masked, secrets), held)"
# Press 4.34.0's New returns early when newHTTPClient could not build a client.
NEW_GUARD_434 = "\tif httpClient == nil {\n\t\treturn c\n\t}\n"
BODY_F1 = ("Body:       c.maskCredentialText(truncateBody(respBody), authHeader),",
           "Body:       c.shownErrorBody(respBody, authHeader),")
BODY_F2 = ("bodyText := truncateBody(respBody)", "bodyText := c.shownErrorBody(respBody, authHeader)")
SUMMARY = ('fmt.Errorf("%s %s: expected JSON, API returned HTML instead of JSON: %s", method, c.displayURL(path, authHeader), summary)',
           'fmt.Errorf("%s %s: expected JSON, API returned HTML instead of JSON: %s", method, c.displayURL(path, authHeader), c.shownSummary(respBody, summary, authHeader))')
MINT = ('return fmt.Errorf("OAuth client_credentials mint: HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(string(body)))',
        'return fmt.Errorf("OAuth client_credentials mint: HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(c.shownErrorBody(body, clientID, clientSecret)))')
MINT_REJECTED = ('return fmt.Errorf("%w: HTTP %d: %s", ErrTokenMintRejected, resp.StatusCode, cliutil.SanitizeErrorBody(string(body)))',
                 'return fmt.Errorf("%w: HTTP %d: %s", ErrTokenMintRejected, resp.StatusCode, cliutil.SanitizeErrorBody(c.shownErrorBody(body, clientID, clientSecret)))')
REFRESH = ('return fmt.Errorf("refreshing access token: HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(truncateBody(body)))',
           'return fmt.Errorf("refreshing access token: HTTP %d: %s", resp.StatusCode, cliutil.SanitizeErrorBody(c.shownErrorBody(body)))')
AUTH_LOGIN = ("cliutil.SanitizeErrorBody(string(body))",
              "cliutil.SanitizeErrorBody(cliutil.FunnelWithhold(string(body), []string{clientSecret}))")

HELPER_ANCHOR = "func truncateBody(b []byte) string {"
HOLD_HELPER = """// holdCredentials records every credential maskCredentialText covers with the
// process-wide funnel (cliutil.FunnelHold), for the error backstops: the CLI's
// error writers and the MCP server's error results. Hand-fix (handfixes.json secret-echo-decoded-funnel).
func (c *Client) holdCredentials() {
	_ = c.maskCredentialText(" ")
}

"""
BODY_HELPER = """// shownErrorBody is the text an error shows of the server's answer (hand-fix
// handfixes.json secret-echo-decoded-funnel): the WHOLE answer is masked and
// checked for an echoed credential before the display cut, then the text that
// will be shown is checked again.
func (c *Client) shownErrorBody(body []byte, extraCredentials ...string) string {
	whole := c.maskCredentialText(string(body), extraCredentials...)
	return c.maskCredentialText(truncateBody([]byte(whole)), extraCredentials...)
}

"""
SUMMARY_HELPER = """// shownSummary funnels a summary of an answer, such as an HTML page's title
// (hand-fix handfixes.json secret-echo-decoded-funnel): the whole answer is
// checked first, then the summary as it will be shown.
func (c *Client) shownSummary(body []byte, summary string, extraCredentials ...string) string {
	if whole := c.maskCredentialText(string(body), extraCredentials...); cliutil.FunnelWithheld(whole) {
		return whole
	}
	return c.maskCredentialText(summary, extraCredentials...)
}

"""
NEW_HOLD = f"\t// {TAG}: the backstops withhold this client's credentials too.\n\tc.holdCredentials()\n"
# The same hold one level in, before the return inside press 4.34.0's guard.
NEW_GUARD_434_HELD = ("\tif httpClient == nil {\n" + "".join("\t" + ln for ln in NEW_HOLD.splitlines(True))
                      + "\t\treturn c\n\t}\n")
AUTH_HOLD = f"\tcliutil.FunnelHold(authHeader) // {TAG}: a minted token is held from here on\n"
AUTH_HEADER_SITES = (  # the three shapes do() resolves its auth header in
    "\tauthHeader, err := c.authHeader(ctx)\n",
    "\tauthHeader := authInfo.Value\n",
    "\t\tauthHeader, err = c.authHeader(ctx)\n\t}\n",
)
ROOT_ANCHOR = "\trootCmd := newRootCmd(&flags)\n"
ROOT_WIRE = f"\t// {TAG}: a command's error passes the funnel before Cobra prints it.\n\tfunnelCommandErrors(rootCmd)\n"
MCP_OPTION = "server.WithToolHandlerMiddleware(funnelToolErrors),"
ENVELOPE = re.compile(r'("error":\s+)err\.Error\(\),')


def a_has(file: str, needle: str) -> dict:
    return {"file": file, "contains": needle, "min_count": 1, "code_only": True}


def a_not(file: str, needle: str) -> dict:
    return {"file": file, "not_contains": needle}


CLIENT = "cli/internal/client/client.go"
# One site, wired differently in different press shapes. A recorded assert for
# one shape of a site goes stale when a reprint moves the connector to another
# shape; ledger_text replaces it only when another shape's assert for the SAME
# site holds in the planned tree.
SITE_SHAPES = (
    ("mask-return", (a_has(CLIENT, RETURN_MARK), a_has(CLIENT, RETURN_MARK_434))),
    ("error-body", (a_has(CLIENT, BODY_F1[1]), a_has(CLIENT, BODY_F2[1]))),
)


TOP_FUNC = re.compile(r"^func [^\n]*\{\n.*?^\}\n", re.S | re.M)
MASK_FUNCS = r"maskCredentialText(?:WithLock)?"   # cipp masks under a lock variant
REQUEST_FUNC = r"doInternal"                       # every generated client sends from here


def code_only(rel: str, text: str) -> str:
    """The comment-stripped source, as check_handfixes.py counts code_only
    asserts: a wiring call left only in a comment is not wiring."""
    stripped, _ = strip_comments(rel, text)
    return text if stripped is None else stripped


def func_spans(text: str, name: str) -> list[tuple[int, int]]:
    """(start, end) of every top-level func whose name matches `name`."""
    out = []
    for m in TOP_FUNC.finditer(text):
        if re.match(r"func (?:\([^)]*\) )?(?:%s)\(" % name, m.group(0)):
            out.append((m.start(), m.end()))
    return out


GO_LITERAL = re.compile(r'"(?:\\.|[^"\\\n])*"|`[^`]*`|\'(?:\\.|[^\'\\\n])*\'')
GO_TOKEN = re.compile(r"\b(func|return|interface|struct)\b|[{}()\n;]")


def own_returns(body: str) -> int:
    """How many `return` statements a Go function body (comments already
    stripped) has outside its function literals: a closure's return never
    leaves the enclosing function, every other return does."""
    return len(own_return_offsets(body))


def own_return_offsets(body: str) -> list[int]:
    """The offset of each of those `return` statements in `body`."""
    body = GO_LITERAL.sub(lambda m: m.group(0)[0] * len(m.group(0)), body)
    stack: list[str] = []
    pending, depth, prev = False, 0, ""
    offsets: list[int] = []
    for m in GO_TOKEN.finditer(body):
        t = m.group(0)
        if t == "func":
            pending, depth = True, 0
        elif t == "(":
            depth += 1
        elif t == ")":
            depth -= 1
        elif t in ("\n", ";") and depth <= 0:
            pending = False  # a func TYPE (no body) ends with its statement
        elif t == "{":
            kind = "type" if prev in ("interface", "struct") else "func" if pending and depth <= 0 else "block"
            pending = pending and kind == "type"
            stack.append(kind)
        elif t == "}" and stack:
            stack.pop()
        elif t == "return" and "func" not in stack:
            offsets.append(m.start())
        prev = t
    return offsets


def unheld_returns(code: str) -> list[int]:
    """Offsets of New's own returns that do not come straight after a
    c.holdCredentials() statement (blank and comment-only lines between are
    allowed, nothing else: a label or a jump target in between could skip it).
    `code` is the comment-stripped body. Empty means every way out of New holds
    the credentials first, whatever the control flow."""
    out = []
    for at in own_return_offsets(code):
        line_start = code.rfind("\n", 0, at) + 1
        before = [ln.strip() for ln in code[:line_start].split("\n") if ln.strip()]
        if code[line_start:at].strip() or not before or before[-1] != "c.holdCredentials()":
            out.append(at)
    return out


def new_guard_434(code: str, raw: str, returns: int) -> bool:
    """Press 4.34.0's New: exactly two own returns, the first inside the
    top-level `if httpClient == nil { return c }` guard and the other the
    top-level `return c`, with no return before the guard. The hold then goes
    right before EACH of the two returns, so no jump can reach a return without
    it. `code` is the comment-stripped body and `raw` the body as written; the
    guard and the top-level return must each occur once in both, so the edits
    land on the statements the code runs."""
    if returns != 2 or code.count(NEW_GUARD_434) != 1 or raw.count(NEW_GUARD_434) != 1:
        return False
    for text in (code, raw):
        if text.count("\n\treturn c\n") + text.startswith("\treturn c\n") != 1:
            return False
    g = code.index(NEW_GUARD_434)
    if g and code[g - 1] != "\n":
        return False
    return own_returns(code[:g]) == 0 and own_returns(code[:g + len(NEW_GUARD_434)]) == 1


class Plan:
    """Every change one connector needs, computed in memory."""

    def __init__(self, cli: Path):
        self.cli = cli
        self.texts: dict[str, str] = {}      # rel -> new text
        self.orig: dict[str, str] = {}       # rel -> text on disk
        self.asserts: list[dict] = []
        self.needs_hand: list[tuple[str, str]] = []  # (site id, what a human must wire)
        self.made: list[str] = []            # edits this run would make

    def text(self, rel: str) -> str:
        if rel not in self.texts:
            p = self.cli / rel
            self.orig[rel] = p.read_text(encoding="utf-8") if p.exists() else ""
            self.texts[rel] = self.orig[rel]
        return self.texts[rel]

    def hand(self, site: str, msg: str) -> None:
        self.needs_hand.append((site, msg))

    def present(self, rel: str, marker: str, scope: str | None = None) -> bool:
        """`marker` is in the code (comments stripped), inside a `scope` func when given."""
        s = self.text(rel)
        spans = func_spans(s, scope) if scope else [(0, len(s))]
        return any(marker in code_only(rel, s[a:b]) for a, b in spans)

    def edit(self, site: str, rel: str, label: str, before: str, after: str, required: bool,
             marker: str, scope: str | None = None) -> bool:
        """Replace `before` by `after` once, only inside a `scope` func when
        given. Already present means `marker` is in the scoped code. True when
        the edit is (now) present. A marker in one function does not make the
        site applied while another function in scope still has the unwired
        anchor: that one is wired (or, when ambiguous, named)."""
        if self.present(rel, marker, scope) and not self.unwired(rel, before, after, scope):
            return True
        s = self.text(rel)
        spans = func_spans(s, scope) if scope else [(0, len(s))]
        if scope and not spans:
            if required:
                self.hand(site, f"{rel}: {label}: no {scope} function to wire")
            return False
        hits = [(a, b) for a, b in spans for _ in range(s[a:b].count(before))]
        if len(hits) == 1:
            a, b = hits[0]
            new = s[:a] + s[a:b].replace(before, after, 1) + s[b:]
            if not any(marker in code_only(rel, new[x:y]) for x, y in (func_spans(new, scope) if scope else [(0, len(new))])):
                self.hand(site, f"{rel}: {label}: the anchor is not in code (a comment?), so the edit would not wire it")
                return False
            self.texts[rel] = new
            self.made.append(f"{rel}: {label}")
            return True
        where = f" in {re.sub(r'[(][?]:[^)]*[)][?]', '', scope)}" if scope else ""
        if len(hits) > 1:
            self.hand(site, f"{rel}: {label}: its anchor appears {len(hits)} times{where}, so the site is ambiguous")
        elif required:
            self.hand(site, f"{rel}: {label}: anchor not found{where}")
        return False

    def unwired(self, rel: str, before: str, after: str, scope: str | None = None) -> int:
        """How many `before` anchors in the scoped code are not yet wired. When
        `after` contains `before` (an insertion after the anchor), a wired
        anchor counts in both, so the difference is what is left."""
        s = self.text(rel)
        want, done = code_only(rel, before), code_only(rel, after)  # compared as code, like the file
        n = 0
        for a, b in (func_spans(s, scope) if scope else [(0, len(s))]):
            code = code_only(rel, s[a:b])
            n += code.count(want) - (code.count(done) if before in after else 0)
        return max(n, 0)

    def edit_shapes(self, site: str, rel: str, label: str, shapes: list[tuple[str, str, str]],
                    required: bool, scope: str | None = None) -> bool:
        """edit() for a site the press has generated in more than one shape:
        `shapes` is [(before, after, marker), ...]. Applied when any marker is
        present; otherwise exactly one shape's anchor may be found, and that
        shape's edit is made. Anchors of two shapes at once is ambiguous; none
        is reported as the first shape's missing anchor."""
        left = [sh for sh in shapes if self.unwired(rel, sh[0], sh[1], scope)]
        if not left and any(self.present(rel, marker, scope) for _, _, marker in shapes):
            return True
        s = self.text(rel)
        spans = func_spans(s, scope) if scope else [(0, len(s))]
        found = [sh for sh in (left or shapes) if any(sh[0] in s[a:b] for a, b in spans)]
        if len(found) > 1:
            self.hand(site, f"{rel}: {label}: anchors of {len(found)} press shapes are present, so the site is ambiguous")
            return False
        before, after, marker = found[0] if found else shapes[0]
        return self.edit(site, rel, label, before, after, required, marker, scope)

    def truthful_asserts(self) -> list[dict]:
        """Only the asserts the planned tree satisfies: an edit that could not
        be made is reported for hand wiring, never claimed in the ledger."""
        return [a for a in self.asserts if assert_holds(self, a)]


def assert_holds(p: Plan, a: dict) -> bool:
    """One handfixes.json assert against the PLANNED tree, with check_handfixes.py's
    semantics (code_only counts comment-stripped source; not_contains is whole-file)."""
    f = a.get("file", "")
    if not f.startswith("cli/") or "<mcp>" in f:
        path = p.cli.parent / f
        text = path.read_text(encoding="utf-8") if path.is_file() else None
        rel = f
    else:
        rel = f[len("cli/"):]
        text = p.text(rel) if (p.cli / rel).exists() or rel in p.texts else None
        text = text if text else None
    if text is None:
        return False
    if "contains" in a:
        hay = code_only(rel, text) if a.get("code_only") is True else text
        if hay.count(a["contains"]) < int(a.get("min_count", 1)):
            return False
    if "not_contains" in a and a["not_contains"] in text:
        return False
    return "contains" in a or "not_contains" in a


def module_path(cli: Path) -> str:
    m = re.search(r"^module (\S+)", (cli / "go.mod").read_text(encoding="utf-8"), re.M)
    if not m:
        raise SystemExit(f"apply_secret_funnel: no module line in {cli / 'go.mod'}")
    return m.group(1)


def mcp_dir(cli: Path) -> str | None:
    hits = sorted(p.parent for p in cli.glob("cmd/*-mcp/main.go")
                  if "server.NewMCPServer(" in p.read_text(encoding="utf-8"))
    return str(hits[0].relative_to(cli)) if len(hits) == 1 else None


def canonical(template: str, module: str) -> str:
    return (CANON / template).read_text(encoding="utf-8").replace("MODULE", module)


def _func_text(block: str) -> str:
    """A helper block without its doc comment: from `func` to the closing brace."""
    return block[block.index("func "):].rstrip("\n") + "\n"


def _arg_line(line: str) -> bool:
    """One whole constructor argument on its own line: not blank, not a
    comment, ends in a comma, parentheses and quotes balanced."""
    t = line.strip()
    return (bool(t) and not t.startswith("//") and t.endswith(",") and "//" not in t
            and t.count("(") == t.count(")") and t.count('"') % 2 == 0)


def plan_connector(cli: Path) -> Plan:
    p = Plan(cli)
    module = module_path(cli)
    mcp = mcp_dir(cli)
    if mcp is None:
        p.hand("mcp-main", "cmd/*-mcp/main.go: no single MCP main.go builds a server.NewMCPServer")

    # 1. Canonical sources.
    for template, dest in FILES:
        if dest.startswith("MCP/"):
            if mcp is None:
                continue
            dest = mcp + dest[3:]
        want = canonical(template, module)
        if p.text(dest) != want:
            p.texts[dest] = want
            p.made.append(f"{dest}: copy the canonical {template}")
    C = "cli/internal/client/client.go"
    p.asserts += [
        a_has("cli/internal/cliutil/secret_funnel.go", "func FunnelEchoes(text string, secrets []string) bool {"),
        a_has("cli/internal/cliutil/secret_funnel_test.go", "func TestFunnelRecognizesEveryEncodingOfAHeldSecret(t *testing.T) {"),
    ]

    # 2. client.go: the masking funnel, the error body, the helpers.
    rel = "internal/client/client.go"
    if not (cli / rel).exists():
        p.hand("client", f"{rel}: missing, so no generated client to wire")
        return p
    held_marker = "held = append(held, value)"
    p.edit_shapes("mask-held", rel, "maskCredentialText holds every masked value",
                  [(HELD_BEFORE, HELD_AFTER, held_marker), (HELD_BEFORE_434, HELD_AFTER_434, held_marker)],
                  True, MASK_FUNCS)
    p.edit_shapes("mask-return", rel, "maskCredentialText ends in cliutil.FunnelWithhold",
                  [(RETURN_BEFORE, RETURN_AFTER, RETURN_MARK), (RETURN_BEFORE_434, RETURN_AFTER_434, RETURN_MARK_434)],
                  True, MASK_FUNCS)
    # Both shapes' return asserts are offered; truthful_asserts keeps only the
    # one the planned tree satisfies.
    p.asserts += [a_has(C, held_marker), a_has(C, RETURN_MARK), a_has(C, RETURN_MARK_434),
                  a_has(C, "func (c *Client) shownErrorBody(body []byte, extraCredentials ...string) string {"),
                  a_not(C, "cliutil.SanitizeErrorBody(string(body))"),
                  a_not(C, "cliutil.SanitizeErrorBody(truncateBody(body))")]
    if p.present(rel, BODY_F1[0], REQUEST_FUNC) or p.present(rel, BODY_F1[1], REQUEST_FUNC):
        p.edit("error-body", rel, "API error body is checked whole before the cut", *BODY_F1, True, BODY_F1[1], REQUEST_FUNC)
        p.asserts += [a_has(C, BODY_F1[1]), a_not(C, "c.maskCredentialText(truncateBody(respBody), authHeader)")]
    else:
        p.edit("error-body", rel, "API error body is checked whole before the cut", *BODY_F2, True, BODY_F2[1], REQUEST_FUNC)
        p.asserts += [a_has(C, BODY_F2[1]), a_not(C, BODY_F2[0])]
    has_summary = p.present(rel, SUMMARY[0], REQUEST_FUNC) or p.present(rel, SUMMARY[1], REQUEST_FUNC)
    if has_summary:
        p.edit("html-summary", rel, "2xx HTML summary is funnelled", *SUMMARY, True, SUMMARY[1], REQUEST_FUNC)
    for site, label, (before, after), need in (("mint", "token mint error body", MINT, ("clientID", "clientSecret")),
                                               ("mint-rejected", "token mint rejection body", MINT_REJECTED, ("clientID", "clientSecret")),
                                               ("refresh", "token refresh error body", REFRESH, ())):
        s = p.text(rel)
        i = s.find(before)
        if i < 0:
            continue
        fn = s[s.rfind("\nfunc ", 0, i) + 1: s.find("\n", s.rfind("\nfunc ", 0, i) + 1)]
        name = re.match(r"func \(c \*Client\) (\w+)\(", fn)
        if not name or any(v not in fn for v in need):
            p.hand(site, f"{rel}: {label}: the enclosing {fn!r} lacks the receiver or {need}")
            continue
        p.edit(site, rel, label, before, after, True, after, name.group(1))

    # The helpers: inserted when absent, and an existing one must be the
    # canonical function exactly (a changed body is not the funnel).
    wanted = [("holdCredentials", HOLD_HELPER), ("shownErrorBody", BODY_HELPER)]
    if has_summary:
        wanted.append(("shownSummary", SUMMARY_HELPER))
    s = p.text(rel)
    missing = ""
    for name, block in wanted:
        spans = func_spans(s, name)
        declared = re.search(r"func \(c \*Client\) %s\(" % name, code_only(rel, s))
        if not spans and declared:
            p.hand("helpers", f"{rel}: {name} is declared in a form the tool cannot delimit (one line?); write it as the canonical function")
        elif not spans:
            missing += block
        elif len(spans) > 1 or s[spans[0][0]:spans[0][1]] != _func_text(block):
            p.hand("helpers", f"{rel}: {name} exists but is not the canonical function; restore it or wire by hand")
    if missing:
        if s.count(HELPER_ANCHOR) != 1:
            p.hand("helpers", f"{rel}: helper methods: '{HELPER_ANCHOR}' appears {s.count(HELPER_ANCHOR)} times")
        else:
            p.texts[rel] = s.replace(HELPER_ANCHOR, missing + HELPER_ANCHOR)
            p.made.append(f"{rel}: the funnel's helper methods")
    if has_summary:
        p.asserts += [a_has(C, "c.shownSummary(respBody, summary, authHeader)"),
                      a_has(C, "func (c *Client) shownSummary(body []byte, summary string, extraCredentials ...string) string {")]
    s = p.text(rel)
    if MINT[1] in s or MINT_REJECTED[1] in s:
        p.asserts.append(a_has(C, "cliutil.SanitizeErrorBody(c.shownErrorBody(body, clientID, clientSecret))"))
    if REFRESH[1] in s:
        p.asserts.append(a_has(C, "cliutil.SanitizeErrorBody(c.shownErrorBody(body)))"))

    # New holds the client's credentials; doInternal holds the resolved auth header.
    # Applied means every way out of New holds first: a hold that only some
    # returns come after (or that a jump can skip) is not the wiring.
    s = p.text(rel)
    m = re.search(r"^func New\(.*?\) \*Client \{\n(.*?)^\}\n", s, re.S | re.M)
    new_code = code_only(rel, m.group(1)) if m else ""
    returns = own_returns(new_code) if m else 0
    if not (m and returns and not unheld_returns(new_code)):
        if m and returns == 1 and m.group(1).count("\n\treturn c\n") + m.group(1).startswith("\treturn c\n") == 1:
            body = m.group(1).replace("\n\treturn c\n", "\n" + NEW_HOLD + "\treturn c\n")
            p.texts[rel] = s[:m.start(1)] + body + s[m.end(1):]
            p.made.append(f"{rel}: New holds the client's credentials")
        elif m and new_guard_434(new_code, m.group(1), returns):
            # The guard's return is unheld (the guard matched as generated); the
            # final return gets a hold only if it has none yet.
            body = m.group(1).replace(NEW_GUARD_434, NEW_GUARD_434_HELD)
            if own_return_offsets(new_code)[-1] in unheld_returns(new_code):
                body = body.replace("\n\treturn c\n", "\n" + NEW_HOLD + "\treturn c\n")
            p.texts[rel] = s[:m.start(1)] + body + s[m.end(1):]
            p.made.append(f"{rel}: New holds the client's credentials before both of its returns (press 4.34.0)")
        else:
            p.hand("new-hold", f"{rel}: New: needs exactly one return, a top-level 'return c' (or press 4.34.0's "
                               "'if httpClient == nil { return c }' guard plus one top-level 'return c'), to hold "
                               "the client's credentials before")
    if not p.edit_shapes("auth-hold", rel, "doInternal holds the resolved auth header",
                         [(shape, shape + AUTH_HOLD, "cliutil.FunnelHold(authHeader)") for shape in AUTH_HEADER_SITES],
                         False, REQUEST_FUNC):
        if not any(site == "auth-hold" for site, _ in p.needs_hand):
            p.hand("auth-hold", f"{rel}: doInternal: the auth header resolution matches none of the known shapes")

    # 3. The cli package: root wiring and every error envelope.
    rel = "internal/cli/root.go"
    if not (cli / rel).exists():
        p.hand("root", f"{rel}: missing")
    else:
        p.edit("root", rel, "Execute wires funnelCommandErrors", ROOT_ANCHOR, ROOT_ANCHOR + ROOT_WIRE, True,
               "funnelCommandErrors(rootCmd)", "Execute")
    envelopes = 0
    for path in sorted((cli / "internal/cli").glob("*.go")):
        if path.name.endswith("_test.go") or path.name.startswith("secret_funnel"):
            continue
        r = str(path.relative_to(cli))
        s = p.text(r)
        new, n = ENVELOPE.subn(r"\1funnelShownError(err),", s)
        if n:
            p.texts[r] = new
            p.made.append(f"{r}: {n} error envelope(s) funnelled")
        if "funnelShownError(err)," in code_only(r, p.texts[r]):
            envelopes += 1
            p.asserts += [a_has("cli/" + r, "funnelShownError(err),"), a_not("cli/" + r, '"error": err.Error(),')]
    if envelopes == 0:
        p.hand("envelope", "internal/cli: no JSON/agent error envelope (\"error\": err.Error()) found to funnel")

    # 4. The MCP server: the outermost tool middleware, after the two real
    # arguments (never after a comment or a split argument).
    if mcp is not None:
        rel = mcp + "/main.go"
        s = p.text(rel)
        if MCP_OPTION not in code_only(rel, s):
            m = re.search(r"server\.NewMCPServer\(\n((\t+)[^\n]*)\n(\t+[^\n]*)\n", s)
            if not m or s.count("server.NewMCPServer(") != 1 or not (_arg_line(m.group(1)) and _arg_line(m.group(3))):
                p.hand("mcp-main", f"{rel}: server.NewMCPServer(name, version, ...) is not two plain argument lines")
            else:
                ind = m.group(2)
                ins = f"{ind}// {TAG}: outermost, so every error result passes the funnel.\n{ind}{MCP_OPTION}\n"
                new = s[:m.end()] + ins + s[m.end():]
                if MCP_OPTION not in code_only(rel, new):
                    p.hand("mcp-main", f"{rel}: the constructor's arguments sit in a comment, so the middleware would not be wired")
                else:
                    p.texts[rel] = new
                    p.made.append(f"{rel}: outermost funnel middleware")
    M = f"cli/{mcp}" if mcp else "cli/cmd/<mcp>"
    p.asserts += [
        a_has("cli/internal/cliutil/secret_funnel.go", "var funnelSteps = []func(string) []string{"),
        a_has("cli/internal/cliutil/secret_funnel.go", "funnelTextStep(funnelIdentityUnescape),"),
        a_has("cli/internal/cliutil/secret_funnel.go", "func FunnelHeld(text string) string {"),
        a_has("cli/internal/cliutil/secret_funnel_test.go", "func TestFunnelCodexRound2(t *testing.T) {"),
        a_has("cli/internal/cli/secret_funnel_cli.go", "func funnelCommandErrors(root *cobra.Command) {"),
        a_has("cli/internal/cli/secret_funnel_cli_test.go", "func TestFunnelCommandErrorsWithholdsAnEchoedCredential(t *testing.T) {"),
        a_has("cli/internal/cli/root.go", "funnelCommandErrors(rootCmd)"),
        a_has(f"{M}/main.go", MCP_OPTION),
        a_has(f"{M}/secret_funnel_mcp.go", "func funnelToolErrors(next server.ToolHandlerFunc) server.ToolHandlerFunc {"),
        a_has(f"{M}/secret_funnel_mcp_test.go", "func TestFunnelToolErrorsWithholdsAnEchoedCredential(t *testing.T) {"),
        a_has(C, "c.holdCredentials()"),
        a_has(C, "cliutil.FunnelHold(authHeader)"),
    ]
    if NEW_GUARD_434_HELD in p.text("internal/client/client.go"):
        # Press 4.34.0's New holds before each of its two returns.
        p.asserts.append({"file": C, "contains": "c.holdCredentials()", "min_count": 2, "code_only": True})

    # 5. The generated `auth login` token exchange.
    rel = "internal/cli/auth.go"
    if (cli / rel).exists():
        s = p.text(rel)
        for m in re.finditer(r"^func mintClientCredentialsToken\(([^)]*)\)[^\n]*\{\n(.*?)^\}\n", s, re.S | re.M):
            body = m.group(2)
            if AUTH_LOGIN[0] not in body:
                continue
            if "clientSecret" not in m.group(1) or body.count(AUTH_LOGIN[0]) != 1:
                p.hand("auth-login", f"{rel}: mintClientCredentialsToken: the token error body has no single clientSecret site to funnel")
                continue
            new_body = body.replace(AUTH_LOGIN[0], AUTH_LOGIN[1])
            p.texts[rel] = s[:m.start(2)] + new_body + s[m.end(2):]
            p.made.append(f"{rel}: auth login token error body funnelled")
        if AUTH_LOGIN[1] in p.text(rel):
            p.asserts += [a_has("cli/" + rel, AUTH_LOGIN[1]), a_not("cli/" + rel, AUTH_LOGIN[0])]

    # 6. Hand-written token exchanges the tool cannot wire.
    for site, msg in unwired_token_sites(cli, p):
        p.hand(site, msg)
    return p


EXCHANGE = re.compile(r"\.Do\(|PostForm\(|http\.Post\(")
FUNNELLED = re.compile(r"FunnelWithhold\(|shownErrorBody\(|RedactTokenBody\(|maskCredentialText\(")
FORMATS = re.compile(r"\b(Errorf|Sprintf|errors\.New)\(")
SERVER_TEXT = re.compile(r"string\((body|data|raw|respBody|b)\)|truncate\w*\(|ErrorDesc|error_description|"
                         r"tokenErrCode\(|, (body|data|raw|msg|desc)\)")


def unwired_token_sites(cli: Path, p: Plan) -> list[tuple[str, str]]:
    """A function that performs a token exchange itself (grant_type plus an
    HTTP call) and formats server text into an error, with no funnel call
    anywhere in it, is a site a human must wire: fold the complete body
    through cliutil.FunnelWithhold with the secrets the exchange sent, at the
    error exit, before any branch formats a field out of it."""
    out = []
    for path in sorted(cli.rglob("*.go")):
        if path.name.endswith("_test.go") or path.name.startswith("secret_funnel"):
            continue
        rel = str(path.relative_to(cli))
        s = p.texts.get(rel)
        if s is None:
            s = path.read_text(encoding="utf-8", errors="replace")
        if "grant_type" not in s:
            continue
        for m in TOP_FUNC.finditer(s):
            body = m.group(0)
            if "grant_type" not in body or not EXCHANGE.search(body) or FUNNELLED.search(body):
                continue
            head = body.split("\n", 1)[0]
            name = re.match(r"func (?:\([^)]*\) )?(\w+)", head)
            site = f"token-exchange:{rel}:{name.group(1) if name else head[5:40]}"
            for n, line in enumerate(body.splitlines(), start=s.count("\n", 0, m.start()) + 1):
                if FORMATS.search(line) and SERVER_TEXT.search(line):
                    out.append((site, f"{rel}:{n}: token exchange {head[5:60]!r} formats server text with no funnel call"))
    return out


# --------------------------------------------------------------------------- #
# The ledger.
# --------------------------------------------------------------------------- #

NEW_ENTRY = {
    "id": ENTRY_ID,
    "class": CLASS,
    "summary": (
        "One decoded output funnel for every server text the client shows, applied by "
        "tools/maintainer/apply_secret_funnel.py: maskCredentialText still masks each held credential as "
        "written and URL-escaped, then withholds the WHOLE text if a held value survives in any view of it "
        "(control characters, JSON and JavaScript escapes, the identity escape, percent, HTML entities, tags, "
        "base64 and hex, searched breadth-first and capped by total work; cliutil.FunnelWithhold, shared "
        "byte-identical across the fleet). The API error body is checked whole before the display cut; the 2xx "
        "HTML summary and the token mint, refresh and auth login error bodies are funnelled when present; the "
        "client holds its credentials when built and its resolved auth header on every request; every command "
        "error before Cobra prints it, every JSON/agent error envelope, and every MCP error result pass the same "
        "funnel over everything the process holds. Success output is never funnelled."
    ),
    "why": (
        "class_harness.py secret-echo drives the built CLI and MCP server against a fake server that reflects each "
        "configured credential in every encoding; before press 4.34.0 the generated masks cover only the raw, "
        "QueryEscape and PathEscape forms (cli-printing-press #5145), so a fresh print fails it. Press 4.34.0 "
        "(#5169) masks encoded forms too; the funnel stays for the connector's hand-written code (token "
        "endpoints, novel commands), which the generated masks never see. Matching encoded needles one at a "
        "time never converged in review, so the fix is a decode-then-check backstop that withholds the whole text "
        "and fails closed (text over 1 MiB or work past the cap is withheld). The withheld notice keeps the auth "
        "reading of an auth failure, so exit codes still classify. Known limits: values under 8 characters are "
        "only masked as written; case-changed, split-and-interleaved, compressed or encrypted echoes are not "
        "reconstructed. A reprint that drops these edits fails class_harness.py secret-echo in CI and these "
        "asserts; re-run tools/maintainer/apply_secret_funnel.py --slug <slug>."
    ),
    "status": "active",
    "spec_encode_followup": (
        "Press-side: cli-printing-press issue #5145 (encoded credential echo). The generated maskCredentialText "
        "should end in a decode-then-check backstop over every held credential, and the error-body and "
        "token-endpoint paths should check the whole body before the display cut."
    ),
}


def detect_format(raw: str, data) -> tuple[int, bool]:
    for ind in (1, 2, 4):
        for ea in (True, False):
            if json.dumps(data, indent=ind, ensure_ascii=ea) + "\n" == raw:
                return ind, ea
    raise SystemExit("apply_secret_funnel: handfixes.json does not round-trip as JSON with 1, 2 or 4-space "
                     "indent; fix its formatting first so this tool cannot rewrite unrelated lines")


def hand_wired(skill: Path) -> dict[str, str]:
    """Sites a human wired by hand and declared on the ledger entry, as
    {"<site id>": "<how it is wired>"}. A declaration without a reason is
    ignored, so a site is never waived by an empty string."""
    path = skill / "handfixes.json"
    if not path.exists():
        return {}
    entry = next((e for e in json.loads(path.read_text(encoding="utf-8")).get("handfixes", [])
                  if e.get("id") == ENTRY_ID), None)
    declared = (entry or {}).get("hand_wired") or {}
    return {k: v for k, v in declared.items() if isinstance(v, str) and v.strip()} if isinstance(declared, dict) else {}


def _same_assert(a: dict, b: dict) -> bool:
    return json.dumps(a, sort_keys=True) == json.dumps(b, sort_keys=True)


def stale_shape_assert(a: dict, plan: "Plan") -> bool:
    """True when `a` is one press shape's assert for a SITE_SHAPES site, it no
    longer holds in the planned tree, and another shape's assert for the same
    site does: a reprint moved the site to a new shape, so `a` describes code
    that no longer exists. Anything else that fails (a hand-added assert, a
    site with no wiring in place) is not stale and stays to fail the proof."""
    for _, shapes in SITE_SHAPES:
        if any(_same_assert(a, s) for s in shapes):
            return (not assert_holds(plan, a)
                    and any(assert_holds(plan, s) for s in shapes if not _same_assert(a, s)))
    return False


def ledger_text(skill: Path, slug: str, asserts: list[dict], plan: "Plan | None" = None) -> tuple[str, str, list[dict]]:
    """(current text, wanted text, stale asserts replaced) of skills/<slug>/handfixes.json.
    Recorded asserts keep their order, so an unchanged connector's ledger is
    byte-identical; only stale_shape_assert() entries are removed."""
    path = skill / "handfixes.json"
    if path.exists():
        raw = path.read_text(encoding="utf-8")
        data = json.loads(raw)
        ind, ea = detect_format(raw, data)
    else:
        raw, ind, ea = "", 1, True
        data = {"slug": slug, "doc": "docs/reprint-survival.md",
                "_comment": "Connector-specific edits a cli-printing-press reprint must NOT clobber. "
                            "check_handfixes.py asserts every marker below is still present (and every banned "
                            "anti-pattern still absent) and fails CI if a reprint dropped one.",
                "handfixes": []}
    entries = data.setdefault("handfixes", [])
    entry = next((e for e in entries if e.get("id") == ENTRY_ID), None)
    if entry is None:
        entry = json.loads(json.dumps(NEW_ENTRY))
        entry["asserts"] = []
        entries.append(entry)
    entry.setdefault("class", CLASS)
    recorded = entry.setdefault("asserts", [])
    stale = [a for a in recorded if plan is not None and isinstance(a, dict) and stale_shape_assert(a, plan)]
    if stale:
        entry["asserts"] = recorded = [a for a in recorded if not any(a is s for s in stale)]
    have = {json.dumps(a, sort_keys=True) for a in recorded}
    for a in asserts:
        if json.dumps(a, sort_keys=True) not in have:
            entry["asserts"].append(a)
            have.add(json.dumps(a, sort_keys=True))
    return raw, json.dumps(data, indent=ind, ensure_ascii=ea) + "\n", stale


# --------------------------------------------------------------------------- #
# Commands.
# --------------------------------------------------------------------------- #

def run_slug(skill: Path, slug: str, check: bool, out=print, prove=None) -> int:
    cli = skill / "cli"
    if not (cli / "go.mod").exists():
        out(f"{slug}: no cli/go.mod, so there is no connector to wire")
        return 1
    plan = plan_connector(cli)
    declared = hand_wired(skill)
    pending = [(site, msg) for site, msg in plan.needs_hand if site not in declared]
    raw, want, stale = ledger_text(skill, slug, plan.truthful_asserts(), plan)
    changes = [rel for rel in plan.texts if plan.texts[rel] != plan.orig.get(rel, "")]
    ledger_change = raw != want
    for a in stale:
        out(f"{slug}: {'would replace' if check else 'replaced'} a stale {ENTRY_ID} assert (the site moved to "
            f"another press shape): {a['file']}: {a['contains']}")
    if check:
        for rel in changes:
            n = sum(1 for ln in difflib.unified_diff(plan.orig.get(rel, "").splitlines(), plan.texts[rel].splitlines(),
                                                     lineterm="", n=0) if ln[:1] in "+-" and ln[:3] not in ("+++", "---"))
            out(f"{slug}: would change cli/{rel} ({n} changed lines)")
        if ledger_change:
            out(f"{slug}: would update handfixes.json ({ENTRY_ID})")
    else:
        for rel in changes:
            dest = cli / rel
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(plan.texts[rel], encoding="utf-8")
        if ledger_change:
            (skill / "handfixes.json").write_text(want, encoding="utf-8")
        for m in plan.made:
            out(f"{slug}: {m}")
        if ledger_change:
            out(f"{slug}: handfixes.json entry {ENTRY_ID} written")
    for site, msg in plan.needs_hand:
        if site in declared:
            out(f"{slug}: hand-wired ({site}): {declared[site]}")
    for site, msg in pending:
        out(f"{slug}: NEEDS HAND WIRING [{site}]: {msg}")
    if pending:
        out(f"{slug}: FAIL: {len(pending)} site(s) need hand wiring. Wire each one, add asserts for your wiring, "
            f"declare it in handfixes.json as \"hand_wired\": {{\"<site id>\": \"<how it is wired>\"}} on the "
            f"{ENTRY_ID} entry, then re-run with --check")
        return 1
    if check and (changes or ledger_change):
        out(f"{slug}: FAIL: not applied; run without --check")
        return 1
    # Success is measured, never inferred from the text. --check stays cheap
    # (the ledger's own checker only); every apply, even one that changes
    # nothing, must also build, vet and pass the secret-echo class test, so a
    # re-run after a hand fix proves the tree again.
    root = skill.parent.parent
    prove = prove or _PROVER[0]
    full = not check
    failed = prove(root, slug, full)
    if failed:
        for f in failed:
            out(f"{slug}: PROOF FAILED: {f}")
        touched = plan.made or ["(no edit this run)"]
        out(f"{slug}: FAIL: the wiring is not proven; sites this run touched: {'; '.join(touched)}")
        return 1
    if not changes and not ledger_change:
        out(f"{slug}: already applied, nothing to change" + (" (proven)" if full else ""))
    else:
        out(f"{slug}: {'applied and proven' if not check else 'would apply'}")
    return 0


def _tail(p: subprocess.CompletedProcess, n: int = 6) -> str:
    lines = (p.stdout + p.stderr).strip().splitlines()
    return " | ".join(lines[-n:]) if lines else f"exit {p.returncode}"


def run_proofs(root: Path, slug: str, full: bool) -> list[str]:
    """The proofs a wired connector must pass, by execution. Returns one line
    per failed proof. Always: check_handfixes.py --slug (the ledger, with the
    checker's own semantics). With `full`: go build ./..., go vet ./..., and
    class_harness.py --slug --class secret-echo, judged by this connector's
    own failing count (a baselined failure is still a failure here)."""
    failed = []
    checker = root / "tools/maintainer/check_handfixes.py"
    if not checker.exists():
        return [f"check_handfixes: {checker} is missing, so the ledger cannot be checked"]
    p = subprocess.run([sys.executable, "-I", str(checker), "--slug", slug], cwd=root,
                       capture_output=True, text=True, timeout=600)
    if p.returncode != 0:
        failed.append(f"check_handfixes.py --slug {slug}: {_tail(p)}")
    if not full:
        return failed
    cli = root / "skills" / slug / "cli"
    if shutil.which("go") is None:
        return failed + ["go is not on PATH, so the edits could not be built"]
    for name, argv in (("go build ./...", ["go", "build", "./..."]), ("go vet ./...", ["go", "vet", "./..."])):
        p = subprocess.run(argv, cwd=cli, capture_output=True, text=True, timeout=1200)
        if p.returncode != 0:
            failed.append(f"{name}: {_tail(p)}")
            return failed  # the class test cannot build what go build could not
    harness = root / "tools/maintainer/class_harness.py"
    if not harness.exists():
        return failed + [f"class_harness: {harness} is missing, so the funnel cannot be exercised"]
    wrong = harness_target_error(root, slug)
    if wrong:
        return failed + [wrong]
    p = subprocess.run([sys.executable, "-I", str(harness), "--slug", slug, "--class", "secret-echo", "-v"],
                       cwd=root, capture_output=True, text=True, timeout=1200)
    bad = harness_verdict(p.stdout, p.returncode, hand_wired(root / "skills" / slug).get(EXERCISE_SITE))
    if bad:
        failed.append(f"class_harness.py --slug {slug} --class secret-echo: {bad}: {_tail(p)}")
    return failed


EXERCISE_SITE = "class-test"  # hand_wired key: how the error path was proven when the class test cannot reach it
HARNESS_SUMMARY = re.compile(r"== class secret-echo: (\d+) failing / (\d+) checked \((\d+) baselined, (\d+) advisory\)")


def harness_target_error(root: Path, slug: str) -> str | None:
    """The class test finds a connector through the registry; it must point at
    the tree this run edited, or its pass proves nothing about it."""
    try:
        entry = json.loads((root / "tools/maintainer/skills.json").read_text(encoding="utf-8"))["skills"][slug]
    except (OSError, ValueError, KeyError, TypeError) as exc:
        return f"class_harness: {slug} has no readable registry entry ({exc.__class__.__name__}), so the class test cannot target it"
    if not isinstance(entry, dict) or entry.get("source_dir", slug) != slug or entry.get("markdown_only"):
        return f"class_harness: the registry does not point the class test at skills/{slug}, so it would not test this tree"
    return None


def harness_verdict(stdout: str, rc: int, waiver: str | None) -> str | None:
    """None when the class test exercised this one connector and it passed. A
    connector the test could not reach (an advisory "not exercised") is not
    proven, unless the ledger declares how its error path was proven instead."""
    m = HARNESS_SUMMARY.search(stdout)
    if rc != 0 or not m:
        return "the class test failed or printed no summary"
    failing, checked, _, advisory = (int(g) for g in m.groups())
    if failing or checked != 1:
        return f"{failing} failing of {checked} checked"
    if advisory and not waiver:
        return (f"not exercised (an advisory, not a pass); prove the error path another way and declare it as "
                f"\"hand_wired\": {{\"{EXERCISE_SITE}\": \"<how>\"}}")
    return None


_PROVER = [run_proofs]  # the self-test swaps in a fixture prover


def check_drift(root: Path, out=print) -> int:
    """Every connector that carries the funnel carries the canonical copies."""
    bad = 0
    for go_mod in sorted(root.glob("skills/*/cli/go.mod")):
        cli = go_mod.parent
        # A carrier is any connector with a funnel file or the ledger entry,
        # so deleting one file never makes the rest unchecked.
        ledger = cli.parent / "handfixes.json"
        artifacts = [cli / d for _, d in FILES if not d.startswith("MCP/")] + list(cli.glob("cmd/*/secret_funnel_mcp*.go"))
        has_entry = False
        if ledger.exists():
            try:
                has_entry = any(isinstance(e, dict) and e.get("id") == ENTRY_ID
                                for e in json.loads(ledger.read_text(encoding="utf-8")).get("handfixes", []))
            except (OSError, ValueError, AttributeError) as exc:
                out(f"{ledger.relative_to(root)}: not readable JSON ({exc.__class__.__name__}), so its funnel entry cannot be checked")
                bad += 1
                continue
        if not any(a.exists() for a in artifacts) and not has_entry:
            continue
        module, mcp = module_path(cli), mcp_dir(cli)
        for template, dest in FILES:
            if dest.startswith("MCP/"):
                if mcp is None:
                    out(f"{cli}: carries the funnel but has no single MCP main.go")
                    bad += 1
                    continue
                dest = mcp + dest[3:]
            p = cli / dest
            if not p.exists() or p.read_text(encoding="utf-8") != canonical(template, module):
                out(f"{p.relative_to(root)}: differs from tools/maintainer/secret_funnel/{template}")
                bad += 1
    if bad:
        out("hint: the funnel ships byte-identical in every connector; change tools/maintainer/secret_funnel/ "
            "and re-run apply_secret_funnel.py --slug <slug> for each connector, never one copy by hand")
        return 1
    return 0


# --------------------------------------------------------------------------- #
# Self-test: fixtures of every generated shape, both directions.
# --------------------------------------------------------------------------- #

REDIRECT_CLOSURE = ("\th.CheckRedirect = func(req *http.Request, via []*http.Request) error {\n\t\tif len(via) >= 10 {\n"
                    "\t\t\treturn errors.New(\"stopped after 10 redirects\")\n\t\t}\n\t\treturn nil\n\t}\n")
# Press 4.34.0's New, as generated (cipp, servosity, ninjaone): the Client
# literal, the cache sweep, the early httpClient guard, the CheckRedirect
# closure, then the one top-level return.
NEW_434 = ("func New(cfg *config.Config) *Client {\n\thttpClient, tlsErr := newHTTPClient(cfg)\n\tc := &Client{\n"
           "\t\tConfig:       cfg,\n\t\tHTTPClient:   httpClient,\n\t\ttlsConfigErr: tlsErr,\n\t}\n"
           "\tc.sweepExpiredCache()\n" + NEW_GUARD_434 + REDIRECT_CLOSURE.replace("\th.", "\thttpClient.") + "\treturn c\n}\n")


def _fixture(root: Path, slug: str, *, f2: bool, auth_site: str, summary: bool, mint: bool,
             auth_login: bool, extra_go: dict | None = None, press434: bool = False) -> Path:
    skill = root / "skills" / slug
    cli = skill / "cli"
    tools = root / "tools" / "maintainer"
    if not (tools / "check_handfixes.py").exists():
        tools.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(Path(__file__).resolve().parent / "check_handfixes.py", tools / "check_handfixes.py")
    (cli / "internal/client").mkdir(parents=True)
    (cli / "internal/cli").mkdir(parents=True)
    (cli / "internal/cliutil").mkdir(parents=True)
    (cli / f"cmd/{slug}-mcp").mkdir(parents=True)
    (cli / "go.mod").write_text(f"module {slug}-pp-cli\n\ngo 1.26\n")
    body = (f"\t\tbodyText := truncateBody(respBody)\n\t\tapiErr := &APIError{{\n\t\t\tBody: c.maskCredentialText(bodyText, authHeader),\n\t\t}}\n"
            if f2 else "\t\tapiErr := &APIError{\n\t\t\tBody:       c.maskCredentialText(truncateBody(respBody), authHeader),\n\t\t}\n")
    client = ("package client\n\nimport (\n\t\"strings\"\n\n\t\"x/internal/cliutil\"\n)\n\n"
              "func New(cfg *config.Config) *Client {\n\tc := &Client{Config: cfg}\n\tif cfg != nil {\n\t\treturn c\n\t}\n\treturn c\n}\n\n"
              "func (c *Client) doInternal(ctx context.Context) {\n" + auth_site + body +
              (f"\t\treturn nil, 0, {SUMMARY[0]}\n" if summary else "") + "}\n\n"
              + (f"func (c *Client) mintClientCredentials(ctx context.Context, clientID, clientSecret string) error {{\n\t{MINT[0]}\n}}\n\n" if mint else "")
              + "func (c *Client) maskCredentialText(text string, extraCredentials ...string) string {\n"
              "\ttype credentialMask struct {\n\t\tneedle      string\n\t\treplacement string\n\t}\n"
              + HELD_BEFORE + "\t\taddMask(value)\n\t}\n" + RETURN_BEFORE + "\n" + HELPER_ANCHOR + "\n\treturn string(b)\n}\n")
    # The auth-site shape in New is the nested `return c` the anchor must skip.
    client = client.replace("\tif cfg != nil {\n\t\treturn c\n\t}\n", "\tif cfg != nil {\n\t\t_ = cfg\n\t}\n")
    if press434:
        old_new = client[client.index("func New("):client.index("func (c *Client) doInternal(")]
        client = (client.replace(old_new, NEW_434 + "\n")
                  .replace(HELD_BEFORE, HELD_BEFORE_434)
                  .replace(RETURN_BEFORE, RETURN_BEFORE_434))
    (cli / "internal/client/client.go").write_text(client)
    (cli / "internal/cli/root.go").write_text("package cli\n\nfunc RootCmd() *cobra.Command {\n\tvar flags rootFlags\n\treturn newRootCmd(&flags)\n}\n\n"
                                              "func Execute() error {\n\tvar flags rootFlags\n" + ROOT_ANCHOR + "\treturn rootCmd.Execute()\n}\n")
    (cli / "internal/cli/helpers.go").write_text("package cli\n\nfunc writeAPIErrorEnvelope(err error) {\n\t_ = map[string]any{\n\t\t\"error\": err.Error(),\n\t\t\"code\":  1,\n\t}\n}\n")
    if auth_login:
        (cli / "internal/cli/auth.go").write_text(
            "package cli\n\nfunc mintClientCredentialsToken(httpClient *http.Client, tokenURL, clientID, clientSecret string) (*tokenResponse, error) {\n"
            "\tform := url.Values{\"grant_type\": {\"client_credentials\"}}\n\tresp, err := httpClient.Do(req)\n\tbody, _ := io.ReadAll(resp.Body)\n"
            "\tif resp.StatusCode != http.StatusOK {\n\t\treturn nil, fmt.Errorf(\"token endpoint returned HTTP %d: %s\", resp.StatusCode, cliutil.SanitizeErrorBody(string(body)))\n\t}\n\treturn nil, nil\n}\n")
    (cli / f"cmd/{slug}-mcp/main.go").write_text(f"package main\n\nfunc main() {{\n\ts := server.NewMCPServer(\n\t\t\"{slug}\",\n\t\tversion,\n\t\tserver.WithToolCapabilities(false),\n\t)\n\t_ = s\n}}\n")
    for rel, text in (extra_go or {}).items():
        (cli / rel).parent.mkdir(parents=True, exist_ok=True)
        (cli / rel).write_text(text)
    return skill


def self_test() -> int:
    ok = True
    # Fixtures cannot be built (no module cache, no registry), so the self-test
    # prover runs the REAL ledger checker and stands in for the build, vet and
    # class test, recording what was asked; a slug in `failing` fails its build.
    asked: dict[str, bool] = {}
    failing: set[str] = set()

    def fixture_prove(root: Path, slug: str, full: bool) -> list[str]:
        asked[slug] = full
        got = run_proofs(root, slug, full=False)
        if full and slug in failing:
            got.append("go build ./...: internal/client/client.go:9: method Client.holdCredentials already declared")
        return got

    _PROVER[0] = fixture_prove

    def expect(cond: bool, msg: str) -> None:
        nonlocal ok
        print(f"  {'ok  ' if cond else 'FAIL'} {msg}")
        ok = ok and cond

    sink: list[str] = []
    quiet = sink.append
    shapes = {
        "press 4.24 (F1 body, := auth header, mint/refresh, auth login)":
            dict(f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=True, auth_login=True),
        "press 4.33.5 (bodyText body, HTML summary)":
            dict(f2=True, auth_site=AUTH_HEADER_SITES[0], summary=True, mint=False, auth_login=False),
        "backup-radar auth shape": dict(f2=True, auth_site=AUTH_HEADER_SITES[1], summary=True, mint=False, auth_login=False),
        "sonicwall auth shape": dict(f2=True, auth_site="\tif x {\n" + AUTH_HEADER_SITES[2], summary=False, mint=False, auth_login=False),
        "press 4.34.0 (secrets + finishCredentialMask, early httpClient return, bodyText body, mint, auth login)":
            dict(f2=True, auth_site=AUTH_HEADER_SITES[0], summary=True, mint=True, auth_login=True, press434=True),
    }
    with tempfile.TemporaryDirectory(prefix="apply-secret-funnel-") as tmp:
        for i, (name, kw) in enumerate(shapes.items()):
            skill = _fixture(Path(tmp), f"acme{i}", **kw)
            cli = skill / "cli"
            sink.clear()
            rc = run_slug(skill, skill.name, check=True, out=quiet)
            expect(rc == 1 and any("would change" in s for s in sink), f"{name}: --check on an unwired tree reports work and fails")
            sink.clear()
            rc = run_slug(skill, skill.name, check=False, out=quiet)
            client = (cli / "internal/client/client.go").read_text()
            p434 = kw.get("press434", False)
            ret_mark = RETURN_MARK_434 if p434 else RETURN_MARK
            holds = [NEW_GUARD_434_HELD, "\tc.holdCredentials()\n\treturn c\n}"] if p434 else ["\tc.holdCredentials()\n\treturn c\n}"]
            wired = all(m in client for m in (ret_mark, "c.shownErrorBody(respBody, authHeader)",
                                               *holds, "cliutil.FunnelHold(authHeader)"))
            new_code = code_only("client.go", client.split("func New(")[1].split("\n}\n")[0] + "\n")
            wired = wired and not unheld_returns(new_code) and own_returns(new_code) == (2 if p434 else 1)
            wired = wired and "held = append(held, value)" in client
            if p434:
                wired = wired and RETURN_MARK not in client and "\tvar secrets []string\n\t// Hand-fix" in client
            wired = wired and "funnelCommandErrors(rootCmd)" in (cli / "internal/cli/root.go").read_text()
            wired = wired and "funnelShownError(err)," in (cli / "internal/cli/helpers.go").read_text()
            wired = wired and MCP_OPTION in (cli / f"cmd/{skill.name}-mcp/main.go").read_text()
            wired = wired and (cli / "internal/cliutil/secret_funnel.go").read_text() == canonical(FILES[0][0], "x")
            wired = wired and f'"{skill.name}-pp-cli/internal/cliutil"' in (cli / "internal/cli/secret_funnel_cli.go").read_text()
            if kw["summary"]:
                wired = wired and SUMMARY[1] in client and "func (c *Client) shownSummary(" in client
            if kw["mint"]:
                wired = wired and MINT[1] in client
            if kw["auth_login"]:
                wired = wired and AUTH_LOGIN[1] in (cli / "internal/cli/auth.go").read_text()
            expect(rc == 0 and wired, f"{name}: apply wires every site")
            ledger = json.loads((skill / "handfixes.json").read_text())
            entry = next(e for e in ledger["handfixes"] if e["id"] == ENTRY_ID)
            expect(entry["class"] == CLASS and len(entry["asserts"]) >= 20, f"{name}: the ledger entry carries the class and its asserts")
            recorded = [a.get("contains") for a in entry["asserts"]]
            expect(ret_mark in recorded and (RETURN_MARK if p434 else RETURN_MARK_434) not in recorded,
                   f"{name}: the ledger asserts this shape's mask-return wiring, not the other shape's")
            before = {p: p.read_bytes() for p in skill.rglob("*") if p.is_file()}
            sink.clear()
            rc = run_slug(skill, skill.name, check=False, out=quiet)
            after = {p: p.read_bytes() for p in skill.rglob("*") if p.is_file()}
            expect(rc == 0 and before == after and any("already applied" in s for s in sink), f"{name}: a re-apply is a no-op")
            sink.clear()
            expect(run_slug(skill, skill.name, check=True, out=quiet) == 0, f"{name}: --check on the wired tree passes")

        # Fail closed: a missing anchor is named and the run fails.
        skill = _fixture(Path(tmp), "broken", f2=False, auth_site="\tauthHeader := someOtherShape()\n", summary=False, mint=False, auth_login=False)
        (skill / "cli/cmd/broken-mcp/main.go").write_text("package main\n\nfunc main() { s := server.NewMCPServer(\"x\", version) }\n")
        sink.clear()
        rc = run_slug(skill, "broken", check=False, out=quiet)
        named = [s for s in sink if "NEEDS HAND WIRING" in s]
        expect(rc == 1 and any("auth header" in s for s in named) and any("NewMCPServer" in s for s in named),
               "unanchorable sites are named and the run fails (auth header, MCP server)")

        # A partial apply never claims an edit it could not make, and a site a
        # human wired is accepted only when declared with a reason.
        led = json.loads((skill / "handfixes.json").read_text())
        claimed = [a["contains"] for e in led["handfixes"] for a in e["asserts"] if "contains" in a]
        expect("cliutil.FunnelHold(authHeader)" not in claimed and "return cliutil.FunnelWithhold(masked, held)" in claimed,
               "a partial apply records only the edits it made")
        entry = next(e for e in led["handfixes"] if e["id"] == ENTRY_ID)
        entry["hand_wired"] = {"auth-hold": "", "mcp-main": "a custom server builder; funnelToolErrors added by hand"}
        (skill / "handfixes.json").write_text(json.dumps(led, indent=1) + "\n")
        sink.clear()
        rc = run_slug(skill, "broken", check=True, out=quiet)
        expect(rc == 1 and any("[auth-hold]" in s for s in sink) and not any("[mcp-main]" in s for s in sink),
               "a declared hand-wired site is accepted; one declared without a reason still fails")
        entry["hand_wired"]["auth-hold"] = "do() holds the header in its own resolver"
        (skill / "handfixes.json").write_text(json.dumps(led, indent=1) + "\n")
        sink.clear()
        expect(run_slug(skill, "broken", check=True, out=quiet) == 0, "with every site wired or declared, --check passes")

        # A hand-written token exchange that formats server text: named until funnelled.
        novel = ("package cli\n\nfunc exchange(clientSecret string) error {\n\tform := url.Values{\"grant_type\": {\"client_credentials\"}}\n"
                 "\tresp, _ := http.DefaultClient.Do(req)\n\tif resp.StatusCode != 200 {\n"
                 "\t\treturn fmt.Errorf(\"token failed: %s: %s\", parsed.Error, parsed.ErrorDescription)\n\t}\n\treturn nil\n}\n")
        skill = _fixture(Path(tmp), "novel", f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False,
                         extra_go={"internal/cli/novel_auth.go": novel})
        sink.clear()
        rc = run_slug(skill, "novel", check=False, out=quiet)
        expect(rc == 1 and any("novel_auth.go" in s and "token exchange" in s for s in sink),
               "a hand-written token exchange formatting server text is named")
        wired_novel = novel.replace("\tif resp.StatusCode != 200 {\n",
                                    "\tif resp.StatusCode != 200 {\n\t\tif w := cliutil.FunnelWithhold(string(body), []string{clientSecret}); cliutil.FunnelWithheld(w) {\n\t\t\treturn errors.New(w)\n\t\t}\n")
        (skill / "cli/internal/cli/novel_auth.go").write_text(wired_novel)
        sink.clear()
        expect(run_slug(skill, "novel", check=False, out=quiet) == 0, "the same exchange, once funnelled by hand, passes")

        # Codex round 1: a marker in a comment, a helper whose body changed, a
        # declaration whose own asserts fail, an anchor outside its function,
        # and a comment taken for a constructor argument must all fail closed.
        def wired_fixture(name, **kw):
            sk = _fixture(Path(tmp), name, **{**dict(f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False,
                                                      mint=False, auth_login=False), **kw})
            run_slug(sk, name, check=False, out=quiet)
            return sk
        sk = wired_fixture("r1commented")
        root = sk / "cli/internal/cli/root.go"
        root.write_text(root.read_text().replace("\tfunnelCommandErrors(rootCmd)\n", "\t// funnelCommandErrors(rootCmd)\n"))
        expect(run_slug(sk, "r1commented", check=True, out=quiet) == 1, "a wiring call left only in a comment is not wired")
        rc = run_slug(sk, "r1commented", check=False, out=quiet)
        expect(rc == 0 and "funnelCommandErrors(rootCmd)" in code_only("root.go", root.read_text()),
               "apply re-wires a call that was left only in a comment")
        sk = wired_fixture("r1helper")
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("\twhole := c.maskCredentialText(string(body), extraCredentials...)\n"
                                             "\treturn c.maskCredentialText(truncateBody([]byte(whole)), extraCredentials...)\n",
                                             "\treturn truncateBody(body)\n"))
        sink.clear()
        expect(run_slug(sk, "r1helper", check=True, out=quiet) == 1 and any("[helpers]" in s for s in sink),
               "a helper whose body no longer matches the canonical form is named")
        sk = _fixture(Path(tmp), "r1declared", f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False)
        root = sk / "cli/internal/cli/root.go"
        root.write_text(root.read_text().replace(ROOT_ANCHOR, "\tcmd := newRootCmd(&flags)\n\tfunnelCommandErrors(cmd)\n")
                        .replace("return rootCmd.Execute()", "return cmd.Execute()"))
        run_slug(sk, "r1declared", check=False, out=quiet)
        led = json.loads((sk / "handfixes.json").read_text())
        ent = next(e for e in led["handfixes"] if e["id"] == ENTRY_ID)
        ent["hand_wired"] = {"root": "Execute wraps cmd by hand"}
        ent["asserts"].append(a_has("cli/internal/cli/root.go", "funnelCommandErrors(cmd)"))
        (sk / "handfixes.json").write_text(json.dumps(led, indent=1) + "\n")
        expect(run_slug(sk, "r1declared", check=True, out=quiet) == 0, "a declared, asserted hand-wired root passes")
        root.write_text(root.read_text().replace("\tfunnelCommandErrors(cmd)\n", ""))
        expect(run_slug(sk, "r1declared", check=True, out=quiet) == 1, "a declaration does not excuse its own assert that now fails")
        sk = _fixture(Path(tmp), "r1scope", f2=False, auth_site="\tauthHeader, err := c.resolveAuth(ctx)\n", summary=False,
                      mint=False, auth_login=False,
                      extra_go={"internal/client/diag.go": "package client\n"})
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text() + "\nfunc (c *Client) diag(ctx context.Context) {\n" + AUTH_HEADER_SITES[0] + "}\n")
        sink.clear()
        rc = run_slug(sk, "r1scope", check=False, out=quiet)
        diag = cl.read_text().split("func (c *Client) diag(")[1]
        expect(rc == 1 and any("[auth-hold]" in s for s in sink) and "FunnelHold" not in diag,
               "an auth-header anchor outside doInternal is not used")
        sk = _fixture(Path(tmp), "r1mcp", f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False)
        main_go = sk / "cli/cmd/r1mcp-mcp/main.go"
        main_go.write_text("package main\n\nfunc main() {\n\ts := server.NewMCPServer(\n\t\t\"acme\",\n\t\t// version,\n\t\tversion,\n\t)\n\t_ = s\n}\n")
        before_main = main_go.read_text()
        sink.clear()
        rc = run_slug(sk, "r1mcp", check=False, out=quiet)
        expect(rc == 1 and any("[mcp-main]" in s for s in sink) and main_go.read_text() == before_main,
               "a comment line is never taken for a server constructor argument")

        # Success is measured: an apply that changed something asks for the full
        # proof (build, vet, class test); --check asks only for the ledger check;
        # a failed proof is a nonzero exit that names it.
        base = dict(f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False)
        sk = _fixture(Path(tmp), "proved", **base)
        run_slug(sk, "proved", check=False, out=quiet)
        expect(asked.get("proved") is True, "an apply that changed something runs the full proof")
        run_slug(sk, "proved", check=True, out=quiet)
        expect(asked.get("proved") is False, "--check runs only the cheap ledger check")
        run_slug(sk, "proved", check=False, out=quiet)
        expect(asked.get("proved") is True, "a re-apply that changes nothing is proven again")
        sk = _fixture(Path(tmp), "unproved", **base)
        failing.add("unproved")
        sink.clear()
        rc = run_slug(sk, "unproved", check=False, out=quiet)
        expect(rc == 1 and any("PROOF FAILED: go build" in s for s in sink) and any("sites this run touched" in s for s in sink),
               "a failed proof is a nonzero exit naming the proof and the sites touched")

        # Codex round 2's six inputs end in a nonzero exit, never "applied".
        sk = _fixture(Path(tmp), "r2oneline", **base)
        run_slug(sk, "r2oneline", check=False, out=quiet)
        cl = sk / "cli/internal/client/client.go"
        t = cl.read_text()
        i = t.index("func (c *Client) holdCredentials() {")
        cl.write_text(t[:i] + 'func (c *Client) holdCredentials() { _ = c.maskCredentialText(" ") }\n' + t[t.index("}\n", i) + 2:])
        sink.clear()
        rc = run_slug(sk, "r2oneline", check=False, out=quiet)
        expect(rc == 1 and any("[helpers]" in s for s in sink) and cl.read_text().count("func (c *Client) holdCredentials()") == 1,
               "round 2 input 1: a one-line helper is refused, never declared twice")
        sk = _fixture(Path(tmp), "r2return", **base)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("\tif cfg != nil {\n\t\t_ = cfg\n\t}\n", "\tif cfg == nil {\n\t\treturn c\n\t}\n"))
        sink.clear()
        expect(run_slug(sk, "r2return", check=False, out=quiet) == 1 and any("[new-hold]" in s for s in sink),
               "round 2 input 2: New with an early return is refused")
        # The press's New carries a CheckRedirect closure: its returns stay in
        # the closure, so New still has one return; a func-typed var is no closure.
        closure = ("\th.CheckRedirect = func(req *http.Request, via []*http.Request) error {\n\t\tif len(via) >= 10 {\n"
                   "\t\t\treturn errors.New(\"stopped after 10 redirects\")\n\t\t}\n\t\treturn nil\n\t}\n")
        sk = _fixture(Path(tmp), "r2closure", **base)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("\tif cfg != nil {\n\t\t_ = cfg\n\t}\n", closure))
        expect(run_slug(sk, "r2closure", check=False, out=quiet) == 0 and "\t}\n" + NEW_HOLD + "\treturn c\n}" in cl.read_text(),
               "New whose only other returns sit in a closure is wired")
        sk = _fixture(Path(tmp), "r2functype", **base)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("\tif cfg != nil {\n\t\t_ = cfg\n\t}\n",
                                             "\tvar hook func() error\n\t_ = hook\n\tif cfg == nil {\n\t\treturn c\n\t}\n"))
        sink.clear()
        expect(run_slug(sk, "r2functype", check=False, out=quiet) == 1 and any("[new-hold]" in s for s in sink),
               "a func-typed var is not a closure: the early return after it is refused")

        # Press 4.34.0: each new anchor fails closed on a near miss.
        b434 = dict(base, f2=True, press434=True)
        early = "\tif cfg == nil {\n\t\treturn c\n\t}\n"
        for n, (label, old, new) in enumerate((
                ("a return before the httpClient guard", "\tc.sweepExpiredCache()\n", "\tc.sweepExpiredCache()\n" + early),
                ("a second early return after the guard", "\treturn c\n}\n", early + "\treturn c\n}\n"),
                ("a guard that is not the httpClient guard", NEW_GUARD_434, early),
                ("the guard repeated inside a block comment", NEW_GUARD_434, "\t/*\n" + NEW_GUARD_434 + "\t*/\n" + NEW_GUARD_434))):
            name = f"n434new{n}"
            sk = _fixture(Path(tmp), name, **b434)
            cl = sk / "cli/internal/client/client.go"
            t = cl.read_text()
            i = t.index("func New(")
            cl.write_text(t[:i] + t[i:].replace(old, new, 1))
            sink.clear()
            rc = run_slug(sk, name, check=False, out=quiet)
            new_body = cl.read_text().split("func New(")[1].split("\n}\n")[0]
            expect(rc == 1 and any("[new-hold]" in s for s in sink) and "c.holdCredentials()" not in new_body,
                   f"press 4.34.0 New with {label} is refused")
        sk = _fixture(Path(tmp), "n434mask", **b434)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("return finishCredentialMask(masked, secrets)", "return finishCredentialMask(masked, nil)"))
        sink.clear()
        rc = run_slug(sk, "n434mask", check=False, out=quiet)
        expect(rc == 1 and any("[mask-return]" in s for s in sink) and "FunnelWithhold(finishCredentialMask" not in cl.read_text(),
               "press 4.34.0 maskCredentialText ending in another finishCredentialMask call is refused")
        sk = _fixture(Path(tmp), "n434both", **base)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text() + "\nfunc (c *Client) maskCredentialTextWithLock(text string) string {\n" + RETURN_BEFORE_434)
        sink.clear()
        rc = run_slug(sk, "n434both", check=False, out=quiet)
        expect(rc == 1 and any("[mask-return]" in s and "ambiguous" in s for s in sink),
               "anchors of two press shapes at once are ambiguous and refused")

        # Codex round 1, input 1: a jump must not bypass the hold. Every own
        # return of New comes straight after a hold, whatever the control flow.
        def new_body(cl: Path) -> str:
            return code_only("client.go", cl.read_text().split("func New(")[1].split("\n}\n")[0] + "\n")
        sk = _fixture(Path(tmp), "r1goto", **b434)
        cl = sk / "cli/internal/client/client.go"
        t = cl.read_text()
        i = t.index("func New(")
        cl.write_text(t[:i] + t[i:].replace(NEW_GUARD_434, "\tif httpClient != nil {\n\t\tgoto ready\n\t}\n" + NEW_GUARD_434, 1)
                      .replace("\treturn c\n}\n", "ready:\n\treturn c\n}\n", 1))
        rc = run_slug(sk, "r1goto", check=False, out=quiet)
        expect(rc == 0 and own_returns(new_body(cl)) == 2 and not unheld_returns(new_body(cl)),
               "round 1 input 1: a goto past the guard still meets a hold before each return")
        sk = _fixture(Path(tmp), "r1before", **b434)
        cl = sk / "cli/internal/client/client.go"
        run_slug(sk, "r1before", check=False, out=quiet)
        cl.write_text(cl.read_text().replace(NEW_GUARD_434_HELD, NEW_HOLD + NEW_GUARD_434))
        sink.clear()
        expect(run_slug(sk, "r1before", check=True, out=quiet) == 1 and any("would change" in s for s in sink),
               "a hold that only sits before the guard is not applied (--check fails)")
        rc = run_slug(sk, "r1before", check=False, out=quiet)
        expect(rc == 0 and not unheld_returns(new_body(cl)) and new_body(cl).count("c.holdCredentials()") == 3,
               "apply then holds before the guard's return too, without a second hold before the last one")
        # Codex round 1, input 2: a marker in another masking function does not
        # hide the active function's unwired anchor (both press shapes).
        for shape_kw, ret_b, ret_m in ((b434, RETURN_BEFORE_434, RETURN_MARK_434), (base, RETURN_BEFORE, RETURN_MARK)):
            nm = "r1unused" + ("434" if shape_kw.get("press434") else "old")
            sk = _fixture(Path(tmp), nm, **shape_kw)
            cl = sk / "cli/internal/client/client.go"
            cl.write_text(cl.read_text() + "\nfunc (c *Client) maskCredentialTextWithLock(masked string) string {\n"
                          "\tvar held, secrets []string\n\t" + ret_m + "\n}\n")
            rc = run_slug(sk, nm, check=False, out=quiet)
            active = cl.read_text().split("func (c *Client) maskCredentialText(")[1].split("\n}\n")[0]
            expect(rc == 0 and ret_m in active and ret_b.split("\n")[-3] + "\n}" not in active + "\n}",
                   f"round 1 input 2 ({'4.34.0' if shape_kw.get('press434') else 'old'} shape): the active function is wired "
                   "although another masking function carries the marker")
        # The same class at a site this commit did not add: a second, unwired
        # auth-header resolution in doInternal is named, not hidden by the first.
        sk = _fixture(Path(tmp), "r1auth", **base)
        run_slug(sk, "r1auth", check=False, out=quiet)
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("func (c *Client) doInternal(ctx context.Context) {\n",
                                             "func (c *Client) doInternal(ctx context.Context) {\n" + AUTH_HEADER_SITES[0], 1))
        sink.clear()
        expect(run_slug(sk, "r1auth", check=True, out=quiet) == 1 and any("[auth-hold]" in s for s in sink),
               "a second unwired auth-header anchor in doInternal fails closed, not hidden by the wired one")

        # A reprint replaces the asserts it made stale, and only those.
        def reprint_to_434(sk: Path, name: str) -> None:
            src = _fixture(Path(tmp) / "reprint-src", name, **b434)
            shutil.copyfile(src / "cli/internal/client/client.go", sk / "cli/internal/client/client.go")
        novel = "package cli\n\nfunc novelToken(body string) string {\n\treturn cliutil.FunnelWithhold(body, nil)\n}\n"
        hand_assert = a_has("cli/internal/cli/novel_token.go", "return cliutil.FunnelWithhold(body, nil)")

        def wired_old(name: str) -> tuple[Path, dict]:
            sk = _fixture(Path(tmp), name, **base, extra_go={"internal/cli/novel_token.go": novel})
            run_slug(sk, name, check=False, out=quiet)
            led = json.loads((sk / "handfixes.json").read_text())
            next(e for e in led["handfixes"] if e["id"] == ENTRY_ID)["asserts"].append(hand_assert)
            (sk / "handfixes.json").write_text(json.dumps(led, indent=1) + "\n")
            return sk, led

        def recorded(sk: Path) -> list:
            led = json.loads((sk / "handfixes.json").read_text())
            return next(e for e in led["handfixes"] if e["id"] == ENTRY_ID)["asserts"]
        sk, _ = wired_old("stale")
        before = recorded(sk)
        expect(a_has(CLIENT, RETURN_MARK) in before and a_has(CLIENT, BODY_F1[1]) in before,
               "an old-shape connector records the old mask-return and error-body asserts")
        reprint_to_434(sk, "stale")
        sink.clear()
        rc = run_slug(sk, "stale", check=False, out=quiet)
        after = recorded(sk)
        replaced = [s for s in sink if "replaced a stale" in s]
        expect(rc == 0 and a_has(CLIENT, RETURN_MARK) not in after and a_has(CLIENT, BODY_F1[1]) not in after
               and a_has(CLIENT, RETURN_MARK_434) in after and a_has(CLIENT, BODY_F2[1]) in after and len(replaced) == 2,
               "a reprint to press 4.34.0 replaces the two stale shape asserts with the new shape's")
        expect(hand_assert in after and [a for a in before if a in after] == [a for a in after if a in before],
               "the reprint keeps a hand-added assert and the order of every kept assert")
        sink.clear()
        expect(run_slug(sk, "stale", check=True, out=quiet) == 0 and any("already applied" in s for s in sink),
               "after the reprint, --check reports already applied")
        sk, _ = wired_old("stalebroken")
        reprint_to_434(sk, "stalebroken")
        cl = sk / "cli/internal/client/client.go"
        cl.write_text(cl.read_text().replace("return finishCredentialMask(masked, secrets)", "return finishCredentialMask(masked, nil)"))
        sink.clear()
        rc = run_slug(sk, "stalebroken", check=False, out=quiet)
        expect(rc == 1 and any("[mask-return]" in s for s in sink) and a_has(CLIENT, RETURN_MARK) in recorded(sk),
               "a stale assert whose site has no wiring in the new shape is kept, and the run fails")
        sk, _ = wired_old("stalehand")
        reprint_to_434(sk, "stalehand")
        (sk / "cli/internal/cli/novel_token.go").unlink()
        sink.clear()
        rc = run_slug(sk, "stalehand", check=False, out=quiet)
        expect(rc == 1 and any("PROOF FAILED: check_handfixes" in s for s in sink) and hand_assert in recorded(sk),
               "a hand-added assert that no longer holds is kept, and the proof fails")
        sk, _ = wired_old("stalecheck")
        reprint_to_434(sk, "stalecheck")
        sink.clear()
        expect(run_slug(sk, "stalecheck", check=True, out=quiet) == 1 and any("would replace a stale" in s for s in sink)
               and a_has(CLIENT, RETURN_MARK) in recorded(sk),
               "--check on a reprinted connector names the stale asserts, changes nothing and fails")
        sk = _fixture(Path(tmp), "r2block", **base)
        root = sk / "cli/internal/cli/root.go"
        root.write_text(root.read_text().replace(ROOT_ANCHOR, "\t/*\n" + ROOT_ANCHOR + "\t*/\n\tcmd := newRootCmd(&flags)\n")
                        .replace("return rootCmd.Execute()", "return cmd.Execute()"))
        sink.clear()
        expect(run_slug(sk, "r2block", check=False, out=quiet) == 1 and any("[root]" in s for s in sink)
               and "funnelCommandErrors" not in root.read_text(),
               "round 2 input 3: an anchor inside a block comment is refused and nothing is written into the comment")
        sk = _fixture(Path(tmp), "r2ctor", **base)
        mg = sk / "cli/cmd/r2ctor-mcp/main.go"
        mg.write_text('package main\n\nfunc main() {\n\ts := server.NewMCPServer(\n\t\t"acme",\n\t\t/* version,\n\t\t*/\n\t\tversion,\n\t)\n\t_ = s\n}\n')
        before_ctor = mg.read_text()
        sink.clear()
        expect(run_slug(sk, "r2ctor", check=False, out=quiet) == 1 and any("[mcp-main]" in s for s in sink)
               and mg.read_text() == before_ctor,
               "round 2 input 4: a block comment among the constructor arguments is refused")
        sk = _fixture(Path(tmp), "r2string", **base)
        root = sk / "cli/internal/cli/root.go"
        root.write_text(root.read_text().replace(ROOT_ANCHOR, "\tcmd := newRootCmd(&flags)\n\t// funnelCommandErrors(cmd)\n")
                        .replace("return rootCmd.Execute()", "return cmd.Execute()"))
        run_slug(sk, "r2string", check=False, out=quiet)
        led = json.loads((sk / "handfixes.json").read_text())
        ent = next(e for e in led["handfixes"] if e["id"] == ENTRY_ID)
        ent["hand_wired"] = {"root": "Execute wraps cmd"}
        ent["asserts"].append({"file": "cli/internal/cli/root.go", "contains": "funnelCommandErrors(cmd)", "min_count": 1, "code_only": "true"})
        (sk / "handfixes.json").write_text(json.dumps(led, indent=1) + "\n")
        sink.clear()
        rc_check = run_slug(sk, "r2string", check=True, out=quiet)
        named = any("PROOF FAILED: check_handfixes" in s for s in sink)
        expect(rc_check == 1 and named and run_slug(sk, "r2string", check=False, out=quiet) == 1,
               "round 2 input 5: a malformed assert is refused by the ledger checker itself (--check and apply)")
        r6 = Path(tmp) / "r6"
        sk = _fixture(r6, "acme", **base)
        run_slug(sk, "acme", check=False, out=quiet)
        for f6 in list((sk / "cli").rglob("secret_funnel*.go")):
            f6.unlink()
        escaped = '"secret-' + chr(92) + 'u0065cho-decoded-funnel"'
        (sk / "handfixes.json").write_text((sk / "handfixes.json").read_text().replace(f'"{ENTRY_ID}"', escaped))
        expect(check_drift(r6, out=quiet) == 1, "round 2 input 6: an escaped entry id is parsed, so the missing copies fail drift")
        (sk / "handfixes.json").write_text("{not json")
        sink.clear()
        expect(check_drift(r6, out=quiet) == 1 and any("not readable JSON" in s for s in sink), "an unreadable ledger fails drift")

        # Codex round 3: the class test must target this tree and exercise it.
        reg = Path(tmp) / "reg"
        (reg / "tools/maintainer").mkdir(parents=True)
        regf = reg / "tools/maintainer/skills.json"
        for meta, want in (({"acme": {}}, None), ({"acme": {"source_dir": "other"}}, "does not point"),
                           ({"acme": {"markdown_only": True}}, "does not point"), ({}, "no readable registry entry")):
            regf.write_text(json.dumps({"skills": meta}))
            got = harness_target_error(reg, "acme")
            expect(got is None if want is None else bool(got) and want in got, f"registry target {meta}: {want or 'accepted'}")
        line = "== class secret-echo: {} failing / {} checked (0 baselined, {} advisory)\n"
        expect(harness_verdict(line.format(0, 1, 0), 0, None) is None, "an exercised, passing class test is a proof")
        expect("not exercised" in (harness_verdict(line.format(0, 1, 1), 0, None) or ""),
               "an advisory (not exercised) class test is not a proof")
        expect(harness_verdict(line.format(0, 1, 1), 0, "a recorded 401 replay test") is None,
               "a not-exercised class test with a declared proof is accepted")
        expect(bool(harness_verdict(line.format(1, 1, 0), 1, "x")) and bool(harness_verdict(line.format(0, 0, 0), 0, None))
               and bool(harness_verdict("", 0, None)), "a failure, an empty run or no summary is never a proof")

        # --list-outputs names the ledger and every file an apply would write.
        sk = _fixture(Path(tmp), "listing", f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False)
        listed = list_outputs(sk, "listing")
        expect("skills/listing/handfixes.json" in listed and "skills/listing/cli/internal/client/client.go" in listed
               and "skills/listing/cli/internal/cliutil/secret_funnel.go" in listed,
               "--list-outputs names the ledger, the copies and the edited files")
        run_slug(sk, "listing", check=False, out=quiet)
        expect(list_outputs(sk, "listing") == ["skills/listing/handfixes.json"], "on a wired tree only the ledger is listed")

        # Drift: a carrier whose copy differs from the canonical source fails.
        drift_root = Path(tmp) / "drift"
        skill = _fixture(drift_root, "acme", f2=False, auth_site=AUTH_HEADER_SITES[0], summary=False, mint=False, auth_login=False)
        run_slug(skill, "acme", check=False, out=quiet)
        expect(check_drift(drift_root, out=quiet) == 0, "a freshly applied connector has no drift")
        f = skill / "cli/internal/cliutil/secret_funnel.go"
        f.write_text(f.read_text() + "\n// a local edit\n")
        sink.clear()
        expect(check_drift(drift_root, out=quiet) == 1 and any("secret_funnel.go" in s for s in sink), "a drifted copy is named")
        f.unlink()
        sink.clear()
        expect(check_drift(drift_root, out=quiet) == 1, "a carrier missing its decoder (backstops left behind) still fails")
    _PROVER[0] = run_proofs
    print("PASS: apply_secret_funnel self-test" if ok else "FAIL: apply_secret_funnel self-test")
    return 0 if ok else 1


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--slug", action="append", help="connector to wire (repeatable)")
    ap.add_argument("--check", action="store_true", help="report what would change and what needs hand wiring; change nothing")
    ap.add_argument("--check-drift", action="store_true", help="fail if any connector's funnel copy differs from the canonical source")
    ap.add_argument("--list-outputs", action="store_true",
                    help="print every repo path an apply would write for --slug (one per line); change nothing")
    ap.add_argument("--self-test", action="store_true", help="prove the tool both ways on fixtures of every generated shape")
    ap.add_argument("--root", default=str(ROOT), help=argparse.SUPPRESS)
    args = ap.parse_args(argv)
    root = Path(args.root)
    if args.self_test:
        return self_test()
    if args.check_drift:
        return check_drift(root)
    if not args.slug:
        ap.error("pass --slug <slug>, --check-drift or --self-test")
    rc = 0
    for slug in args.slug:
        if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", slug):
            ap.error(f"invalid slug {slug!r}")
        if args.list_outputs:
            for path in list_outputs(root / "skills" / slug, slug):
                print(path)
            continue
        rc |= run_slug(root / "skills" / slug, slug, args.check)
    return rc


def list_outputs(skill: Path, slug: str) -> list[str]:
    """Every repo-relative path an apply would write, so a caller (onboard.py)
    can refuse an ignored one before anything is written: git status never
    lists an ignored file, so it would escape the commit and the rollback."""
    out = {f"skills/{slug}/handfixes.json"}
    if (skill / "cli" / "go.mod").exists():
        plan = plan_connector(skill / "cli")
        out |= {f"skills/{slug}/cli/{rel}" for rel in plan.texts if plan.texts[rel] != plan.orig.get(rel, "")}
    return sorted(out)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
