#!/usr/bin/env python3
"""Generate the docs site's agent-facing files from the catalog, the skills
registry, the skills' SKILL.md files and the docs pages:

  docs/llms.txt                        compact index: one markdown link per page
  docs/llms-full.txt                   expanded corpus
  docs/_markdown_copies/**.md          a clean Markdown copy of each page
  docs/.well-known/agent-skills/index.json   Agent Skills discovery index (v0.2.0)
  docs/.well-known/ard.json            ARD manifest (spec section 5.1) listing the same Skills
  docs/.well-known/ai-catalog.json     the same entries at ARD's predecessor path

Why two llms files: llms.txt is the short, skimmable index AI search engines
fetch first (one linked line per page); llms-full.txt is the full corpus they
pull when they want depth (per-connector outcomes, instead/say examples, FAQs,
governance summary, plus the WHY pillars and the glossary definition).

The Markdown copies: docs/_markdown_copies is the Jekyll collection
`markdown_copies` (docs/_config.yml). Its files carry no front matter, so Jekyll
copies them byte for byte: docs/_markdown_copies/skills/acronis.md is served at
/skills/acronis.md next to the HTML page /skills/acronis/. A collection is used
because GitHub Pages' optional-front-matter plugin would also render a plain
.md file elsewhere in docs/ as an HTML page. Every link inside a copy is
absolute. A page whose Liquid this script cannot render (the home page, the
connector hub and the receipts wall loop over site data) gets no copy, and
llms.txt links its HTML page instead.

Source of truth, in order of preference per connector:
  1. skills/<slug>/page.json  - structured page data, when the pipeline emits it.
  2. docs/skills/<slug>.md     - the rendered page body, parsed as a fallback.

Output is DETERMINISTIC: stable ordering (skills sorted by slug), no timestamps,
no machine-specific paths. verify_all.sh regenerates every output and diffs it
against the committed copy (same drift gate as the catalog), and catalog.yml
regenerates them on main, so a stale output fails CI or is replaced.

Pure stdlib. Run locally:  python3 tools/maintainer/build-llms.py
Self-check:                python3 tools/maintainer/build-llms.py --self-test
"""

from __future__ import annotations

import hashlib
from html import unescape
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/maintainer module)

ROOT = registry.ROOT
DOCS = ROOT / "docs"
SKILLS_DIR = registry.SKILLS_DIR
CATALOG = ROOT / "catalog.json"
LLMS = DOCS / "llms.txt"
LLMS_FULL = DOCS / "llms-full.txt"
COPIES = DOCS / "_markdown_copies"
INCLUDES = DOCS / "_includes"
AGENT_SKILLS_INDEX = DOCS / ".well-known" / "agent-skills" / "index.json"
AI_CATALOG = DOCS / ".well-known" / "ai-catalog.json"
ARD = DOCS / ".well-known" / "ard.json"

SITE = "https://msp-skills.compoundingteams.com"
SITE_HOST = "msp-skills.compoundingteams.com"
REPO_URL = "https://github.com/servosity/msp-skills"
RAW_BASE = "https://raw.githubusercontent.com/servosity/msp-skills/main"
# https://github.com/cloudflare/agent-skills-discovery-rfc (v0.2.0). The URI is
# an opaque version identifier, not a document clients fetch.
AGENT_SKILLS_SCHEMA = "https://schemas.agentskills.io/discovery/0.2.0/schema.json"
# ai-catalog media type for "an Agent Skill defined in Markdown" (ARD spec 3.3).
SKILL_MD_MEDIA_TYPE = 'text/markdown; profile="urn:air:agent-skills"'
LLMS_MAX_CHARS = 50_000  # afdocs llms-txt-size: agents truncate past this
# The directive afdocs and agents look for near the top of every copy.
COPY_DIRECTIVE = ("> For AI agents: every page on this site is listed, with links to its Markdown "
                  "copy, at {site}/llms.txt. This is the Markdown copy of {page}")


# --------------------------------------------------------------------------- #
# Front-matter + markdown parsing helpers (pure stdlib, no yaml dep).
# --------------------------------------------------------------------------- #
def split_front_matter(text: str) -> tuple[dict, str]:
    """Return (front_matter_dict, body). Parses simple scalars plus the
    `faqs:` list (items shaped `- q: ...` / `a: ...`). Other nested structures
    are ignored. Good enough for the docs/skills/*.md pages we read."""
    fm: dict = {}
    body = text
    if text.startswith("---"):
        end = text.find("\n---", 3)
        if end != -1:
            block = text[3:end]
            body = text[end + 4 :]
            fm = _parse_fm_block(block)
    return fm, body


def _parse_fm_block(block: str) -> dict:
    fm: dict = {}
    faqs: list[dict] = []
    in_faqs = False
    cur: dict | None = None
    for line in block.splitlines():
        if re.match(r"^faqs:\s*$", line):
            in_faqs = True
            fm["faqs"] = faqs
            continue
        if in_faqs:
            mq = re.match(r"^\s*-\s*q:\s*(.*)$", line)
            ma = re.match(r"^\s*a:\s*(.*)$", line)
            if mq:
                cur = {"q": _unquote(mq.group(1)), "a": ""}
                faqs.append(cur)
                continue
            if ma and cur is not None:
                cur["a"] = _unquote(ma.group(1))
                continue
            # A non-list, non-indented line ends the faqs block.
            if line and not line[0].isspace() and not line.startswith("-"):
                in_faqs = False
            else:
                continue
        m = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if m and (line and not line[0].isspace()):
            fm[m.group(1)] = _unquote(m.group(2).strip())
    return fm


def _unquote(v: str) -> str:
    v = v.strip()
    if (v.startswith('"') and v.endswith('"')) or (v.startswith("'") and v.endswith("'")):
        return v[1:-1]
    return v


def plain_text(v: str) -> str:
    """A front-matter string as readers should see it: YAML's \\uXXXX escapes
    decoded, and the dashes this repo never commits written as hyphens."""
    if "\\u" in v:
        try:
            v = json.loads('"' + re.sub(r'(?<!\\)"', '\\"', v) + '"')
        except ValueError:
            pass
    return v.replace("\u2014", " - ").replace("\u2013", "-").replace("  ", " ").strip()


def strip_md(s: str) -> str:
    """Flatten inline markdown to plain text for the corpus."""
    s = re.sub(r"`([^`]*)`", r"\1", s)
    s = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", s)  # link -> text
    s = re.sub(r"\{:[^}]*\}", "", s)  # kramdown attrs
    s = re.sub(r"[*_]{1,3}", "", s)  # emphasis
    s = s.replace("&ldquo;", '"').replace("&rdquo;", '"').replace("&nbsp;", " ")
    return s.strip()


def section(body: str, heading: str) -> str:
    """Return the text under a `## heading` up to the next `## ` heading."""
    pat = re.compile(
        rf"^##\s+{re.escape(heading)}\s*$(.*?)(?=^##\s|\Z)", re.DOTALL | re.MULTILINE
    )
    m = pat.search(body)
    return m.group(1).strip() if m else ""


def parse_table(block: str) -> list[tuple[str, str]]:
    """Parse a 2-column markdown table -> [(col1, col2)], skipping header +
    separator rows."""
    rows: list[tuple[str, str]] = []
    for line in block.splitlines():
        line = line.strip()
        if not line.startswith("|"):
            continue
        cells = [c.strip() for c in line.strip("|").split("|")]
        if len(cells) < 2:
            continue
        if set("".join(cells).replace(" ", "")) <= set("-:"):  # separator
            continue
        c1, c2 = strip_md(cells[0]), strip_md(cells[1])
        if c1.lower() in ("question your msp keeps asking", "outcome") or not c1:
            continue
        rows.append((c1, c2))
    return rows


# --------------------------------------------------------------------------- #
# Per-connector data assembly.
# --------------------------------------------------------------------------- #
def load_page_data(slug: str, meta: dict, cat: dict) -> dict:
    """Return a dict with: name, tagline, category, badge, url, install,
    intro, outcomes [(q, cmd)], faqs [(q, a)], governance_summary."""
    page_json = registry.skill_path(slug) / "page.json"
    data: dict = {
        "slug": slug,
        "name": meta.get("display_name", slug.title()),
        "tagline": meta.get("tagline", ""),
        "category": meta.get("category", ""),
        "badge": badge_state(meta),
        "url": f"{SITE}/skills/{slug}/",
        "install": cat.get("install_skill_one_liner", ""),
        "intro": "",
        "outcomes": [],
        "faqs": [],
        "governance_summary": "",
    }
    if page_json.exists():
        pj = json.loads(page_json.read_text(encoding="utf-8"))
        # page.json's real keys are outcome_intro / question / command (the
        # schema render_docs_page.py consumes). The old q/cmd/intro reads
        # silently returned empty strings and the fallback parser papered
        # over it - keep the schema in exactly one shape.
        data["intro"] = pj.get("outcome_intro", data["intro"])
        data["outcomes"] = [
            (o.get("question", ""), o.get("command", ""))
            for o in pj.get("outcomes", [])
        ]
        data["faqs"] = [(f.get("q", ""), f.get("a", "")) for f in pj.get("faqs", [])]
        data["governance_summary"] = pj.get("governance_summary", "")
        return data

    # Fallback: parse the rendered docs page.
    md = DOCS / "skills" / f"{slug}.md"
    if md.exists():
        fm, body = split_front_matter(md.read_text(encoding="utf-8"))
        # Intro = first prose paragraph after the H1 + banner blockquote + the
        # badge notice + the vocab one-liner.
        data["intro"] = first_intro_paragraph(body)
        data["outcomes"] = parse_table(section(body, "What it does"))
        data["faqs"] = [(f["q"], f["a"]) for f in fm.get("faqs", [])]
        gov = section(body, "Safety model")
        if gov:
            # Compress the safety table to a one-line summary.
            data["governance_summary"] = summarize_safety(gov)
    return data


def first_intro_paragraph(body: str) -> str:
    blocks = re.split(r"\n\s*\n", body)
    for b in blocks:
        s = b.strip()
        if not s:
            continue
        first = s.splitlines()[0].lstrip()
        if first.startswith(("#", ">", "<", "{%", "{{", "|", "---", "***")):
            continue
        # Skip the badge notice (both states) + the "New to the term?" vocab
        # one-liner. Badge prefixes match render_docs_page.py's badge strings.
        if s.startswith(
            ("**Awaiting live verification**", "**Passes all 4 mechanical gates**",
             "**✓ Live-verified", "New to the term?")
        ):
            continue
        # Skip pure button/link rows.
        stripped = re.sub(r"!?\[[^\]]*\]\([^)]*\)", "", s)
        stripped = re.sub(r"\{:[^}]*\}", "", stripped).replace("&nbsp;", " ").strip()
        if len(stripped) <= 3:
            continue
        return strip_md(s.replace("\n", " "))
    return ""


def summarize_safety(gov: str) -> str:
    tiers = []
    for c1, c2 in parse_table(gov):
        if c1.lower() in ("tier",):
            continue
        tiers.append(c1)
    if tiers:
        return "Permission tiers: " + ", ".join(tiers) + "."
    # No table? take the first sentence-ish line.
    for line in gov.splitlines():
        line = strip_md(line).strip()
        if line and not line.startswith("|"):
            return line
    return ""


def badge_state(meta: dict) -> str:
    lv = meta.get("live_verified") or {}
    # The canonical verified value is "live-verified" (verify_live.py writes it;
    # build-catalog.py reads it) - not "verified". Named verification: say WHO
    # confirmed it against a production tenant, so AI engines can cite a real
    # verifier instead of a bare badge.
    if isinstance(lv, dict) and lv.get("status") == "live-verified":
        by = lv.get("verified_by") or "a real MSP"
        date = lv.get("date")
        return f"Live-verified by {by}" + (f" ({date})" if date else "")
    return "Awaiting live verification"


# --------------------------------------------------------------------------- #
# WHY pillars + glossary definition (read from the rendered pages).
# --------------------------------------------------------------------------- #
def why_pillars() -> list[tuple[str, str]]:
    md = DOCS / "why-msp-skills.md"
    if not md.exists():
        return []
    _, body = split_front_matter(md.read_text(encoding="utf-8"))
    out: list[tuple[str, str]] = []
    for m in re.finditer(r"^##\s+(.+?)\s*$(.*?)(?=^##\s|\Z)", body, re.DOTALL | re.MULTILINE):
        head = strip_md(m.group(1))
        # First prose paragraph of the pillar.
        para = ""
        for b in re.split(r"\n\s*\n", m.group(2)):
            s = b.strip()
            if not s or s.startswith(("#", ">", "<", "{%", "|", "-")):
                continue
            para = strip_md(s.replace("\n", " "))
            break
        out.append((head, para))
    return out


def glossary_definition() -> str:
    md = DOCS / "what-is-an-mcp-server.md"
    if not md.exists():
        return ""
    _, body = split_front_matter(md.read_text(encoding="utf-8"))
    for b in re.split(r"\n\s*\n", body):
        s = b.strip()
        if not s or s.startswith(("#", ">", "<", "{%", "|")):
            continue
        return strip_md(s.replace("\n", " "))
    return ""


# --------------------------------------------------------------------------- #
# The site's pages, and the clean Markdown copy of each.
# --------------------------------------------------------------------------- #
class Unrenderable(Exception):
    """A page uses Liquid this script does not render; it gets no copy."""


def page_url(rel: Path, fm: dict) -> str:
    """The URL Jekyll serves a docs page at (permalink: pretty)."""
    if fm.get("permalink"):
        return fm["permalink"]
    parts = list(rel.with_suffix("").parts)
    if parts and parts[-1] == "index":
        parts = parts[:-1]
    return "/" + "".join(p + "/" for p in parts)


def copy_path(url: str) -> str:
    """Where a page's Markdown copy is served: /skills/acronis/ -> /skills/acronis.md,
    / -> /index.md. The first form agents (and afdocs) try for a page URL."""
    return "/index.md" if url == "/" else url.rstrip("/") + ".md"


def first_h1(body: str) -> str:
    in_fence = False
    for line in body.splitlines():
        if line.lstrip().startswith(("```", "~~~")):
            in_fence = not in_fence
        elif not in_fence and line.startswith("# "):
            return strip_md(line[2:])
    return ""


def site_pages() -> dict[str, dict]:
    """Every docs page Jekyll renders, keyed by URL: the same set the sitemap
    lists. Skips Jekyll's own folders (_layouts, _markdown_copies, ...)."""
    pages: dict[str, dict] = {}
    for md in sorted(DOCS.rglob("*.md")):
        rel = md.relative_to(DOCS)
        if any(part.startswith(("_", ".")) for part in rel.parts):
            continue
        if rel.parts[0] in ("node_modules", "vendor") or rel == Path("README.md"):
            continue  # docs/_config.yml `exclude`
        text = md.read_text(encoding="utf-8")
        fm, body = split_front_matter(text)
        if fm.get("published") == "false":
            continue
        url = page_url(rel, fm)
        title = first_h1(body) or plain_text(fm.get("title", "")).split(" | ")[0]
        pages[url] = {"src": md, "url": url, "fm": fm, "body": body, "title": title,
                      "description": plain_text(fm.get("description", ""))}
    return pages


_INCLUDE_RE = re.compile(r"\{%-?\s*include\s+([\w.-]+)(.*?)-?%\}", re.S)
_ENTITIES = {"&nbsp;": " ", "&rarr;": "→", "&larr;": "←", "&middot;": "·",
             "&ldquo;": "“", "&rdquo;": "”", "&lsquo;": "‘", "&rsquo;": "’",
             "&hellip;": "...", "&mdash;": " - ", "&ndash;": "-"}


def decode_entities(s: str) -> str:
    for ent, ch in _ENTITIES.items():
        s = s.replace(ent, ch)
    return s


def html_to_markdown(html: str) -> str:
    """The few inline HTML shapes the docs pages and includes use, as Markdown.
    Anything else (a placeholder like <TENANT_ID>) is left exactly as written."""
    s = re.sub(r"<video\b([^>]*)>(.*?)</video>", _video_link, html, flags=re.S)
    s = re.sub(r'<a\s[^>]*?href="([^"]*)"[^>]*>(.*?)</a>', r"[\2](\1)", s, flags=re.S)
    s = re.sub(r"<code>(.*?)</code>", lambda m: f"`{unescape(m.group(1))}`", s, flags=re.S)
    s = re.sub(r"</?(?:strong|b)>", "**", s)
    s = re.sub(r"</?(?:em|i)>", "*", s)
    s = re.sub(r"</p>", "\n\n", s)
    s = re.sub(r"</?(?:p|div)\b[^>]*>", "\n", s)
    s = re.sub(r"</?(?:span|sub|sup)\b[^>]*>", "", s)
    s = re.sub(r"<br\s*/?>", "\n", s)
    return decode_entities(s)


def _video_link(m: re.Match) -> str:
    src = re.search(r'\ssrc="([^"]*)"', m.group(1))
    label = re.search(r"<a\s[^>]*>(.*?)</a>", m.group(2), re.S)
    return f"[{label.group(1).strip() if label else 'Watch the video'}]({src.group(1) if src else ''})"


def render_include(name: str, args: str) -> str:
    """Render one {% include %} the way Jekyll would, for includes built only
    from `include.<param>` values and `if include.<param>` blocks."""
    path = INCLUDES / name
    if not path.is_file():
        raise Unrenderable(name)
    tpl = path.read_text(encoding="utf-8")
    tpl = re.sub(r"\{%-?\s*comment\s*-?%\}.*?\{%-?\s*endcomment\s*-?%\}", "", tpl, flags=re.S)
    params = dict(re.findall(r'(\w+)\s*=\s*"([^"]*)"', args))
    tpl = re.sub(r"\{%-?\s*if\s+include\.(\w+)\s*-?%\}(.*?)\{%-?\s*endif\s*-?%\}",
                 lambda m: m.group(2) if params.get(m.group(1)) else "", tpl, flags=re.S)
    tpl = re.sub(r"\{\{-?\s*include\.(\w+)\s*-?\}\}", lambda m: params.get(m.group(1), ""), tpl)
    if "{%" in tpl or "{{" in tpl:
        raise Unrenderable(name)
    text = html_to_markdown(tpl)
    lines = [ln.strip() for ln in text.splitlines()]
    return re.sub(r"\n{3,}", "\n\n", "\n".join(lines)).strip()


def _split_fences(body: str) -> list[tuple[bool, str]]:
    """[(is_code, text)] - fenced code blocks pass through a copy untouched."""
    chunks: list[tuple[bool, str]] = []
    buf: list[str] = []
    fence = ""
    for line in body.split("\n"):
        mark = re.match(r"^\s*(`{3,}|~{3,})", line)
        if fence:
            buf.append(line)
            if mark and mark.group(1)[0] == fence[0] and len(mark.group(1)) >= len(fence) \
                    and not line.strip()[len(mark.group(1)):].strip():
                chunks.append((True, "\n".join(buf)))
                buf, fence = [], ""
        elif mark:
            if buf:
                chunks.append((False, "\n".join(buf)))
            buf, fence = [line], mark.group(1)
        else:
            buf.append(line)
    if buf:
        chunks.append((bool(fence), "\n".join(buf)))
    return chunks


class LinkResolver:
    """Every link in a copy becomes absolute, so it still works once the copy
    is read away from this site: site paths -> the page's Markdown copy when
    there is one (else its HTML page), files outside docs/ -> GitHub."""

    def __init__(self, copies: dict[str, str], pages: dict[str, dict]):
        self.copies = copies
        self.by_src = {p["src"].resolve(): url for url, p in pages.items()}

    def site(self, path: str, frag: str) -> str:
        if not frag:
            for url in (path, path + "/"):
                if url in self.copies:
                    return SITE + self.copies[url]
        return SITE + path + (f"#{frag}" if frag else "")

    def __call__(self, target: str, src: Path) -> str:
        if target.startswith(("http://", "https://", "mailto:", "tel:")) or not target:
            return target
        if target.startswith("#"):  # this page's own heading, on its HTML page
            return SITE + self.by_src.get(src.resolve(), "/") + target
        path, _, frag = target.partition("#")
        if path.startswith("/"):
            return self.site(path, frag)
        full = (src.parent / path).resolve()
        if full in self.by_src:
            return self.site(self.by_src[full], frag)
        try:
            in_docs = full.relative_to(DOCS.resolve())
            return SITE + "/" + in_docs.as_posix() + (f"#{frag}" if frag else "")
        except ValueError:
            pass
        try:
            rel = full.relative_to(ROOT.resolve()).as_posix()
        except ValueError:
            return target
        kind = "tree" if full.is_dir() else "blob"
        return f"{REPO_URL}/{kind}/main/{rel}" + (f"#{frag}" if frag else "")


_LINK_RE = re.compile(r"(!?\[(?:[^\[\]]|\[[^\[\]]*\])*\]\()([^)\s]+)((?:\s+\"[^\"]*\")?\))")


def _prose_to_markdown(text: str, src: Path, resolve: LinkResolver) -> str:
    spans: list[str] = []

    def hold(m: re.Match) -> str:
        spans.append(m.group(0))
        return f"\x00{len(spans) - 1}\x00"

    text = re.sub(r"(`+)(?:(?!\1).)+?\1", hold, text)  # inline code stays verbatim
    text = re.sub(r"\{:[^}\n]*\}", "", text)  # kramdown attribute lists ({: .btn})
    text = html_to_markdown(text)
    text = _LINK_RE.sub(lambda m: m.group(1) + resolve(m.group(2), src) + m.group(3), text)
    return re.sub(r"\x00(\d+)\x00", lambda m: spans[int(m.group(1))], text)


def markdown_copy(page: dict, resolve: LinkResolver) -> str:
    """A page body as clean, self-contained Markdown, or Unrenderable."""
    body = _INCLUDE_RE.sub(lambda m: render_include(m.group(1), m.group(2)), page["body"])
    out = []
    for is_code, chunk in _split_fences(body):
        if is_code:
            out.append(chunk)
            continue
        if "{%" in chunk or "{{" in chunk:
            raise Unrenderable(str(page["src"].relative_to(DOCS)))
        out.append(_prose_to_markdown(chunk, page["src"], resolve))
    text = "\n".join(out)
    text = "\n".join(line.rstrip() for line in text.split("\n"))
    text = re.sub(r"\n{3,}", "\n\n", text).strip("\n")
    head = COPY_DIRECTIVE.format(site=SITE, page=SITE + page["url"])
    return f"{head}\n\n{text}\n"


def build_copies(pages: dict[str, dict]) -> tuple[dict[str, str], dict[str, str]]:
    """(copies: page URL -> copy path, files: copy path -> text). A copy is
    planned for every page first so links between copies resolve; a page whose
    Liquid cannot be rendered is then dropped, and links resolve again."""
    copies = {url: copy_path(url) for url in pages}
    while True:
        resolve = LinkResolver(copies, pages)
        files: dict[str, str] = {}
        dropped = []
        for url, page in pages.items():
            if url not in copies:
                continue
            try:
                files[copies[url]] = markdown_copy(page, resolve)
            except Unrenderable:
                dropped.append(url)
        if not dropped:
            return copies, files
        for url in dropped:
            del copies[url]


def write_copies(files: dict[str, str]) -> None:
    """The folder is wholly generated: a copy whose page is gone is deleted."""
    for old in sorted(COPIES.rglob("*.md")) if COPIES.exists() else []:
        if "/" + old.relative_to(COPIES).as_posix() not in files:
            old.unlink()
    for path, text in files.items():
        dest = COPIES / path.lstrip("/")
        dest.parent.mkdir(parents=True, exist_ok=True)
        if not dest.exists() or dest.read_text(encoding="utf-8") != text:
            dest.write_text(text, encoding="utf-8")
    for d in sorted((p for p in COPIES.rglob("*") if p.is_dir()), reverse=True):
        if not any(d.iterdir()):
            d.rmdir()


# --------------------------------------------------------------------------- #
# Skills discovery: /.well-known/agent-skills/index.json and ai-catalog.json.
# --------------------------------------------------------------------------- #
def skill_md_fields(path: Path) -> dict[str, str]:
    """`name` and `description` from a SKILL.md's YAML front matter: plain,
    single- and double-quoted scalars, and folded (>) or literal (|) blocks."""
    text = path.read_text(encoding="utf-8")
    m = re.match(r"^---\n(.*?)\n---", text, re.S)
    lines = m.group(1).split("\n") if m else []
    out: dict[str, str] = {}
    for i, line in enumerate(lines):
        km = re.match(r"^(name|description):\s*(.*)$", line)
        if not km:
            continue
        key, val = km.group(1), km.group(2).strip()
        if val[:1] in (">", "|"):
            block = []
            for nxt in lines[i + 1:]:
                if nxt.strip() and not nxt[:1].isspace():
                    break
                block.append(nxt.strip())
            if val[0] == ">":
                paras = "\n".join(block).strip().split("\n\n")
                val = "\n".join(" ".join(p.split("\n")) for p in paras)
            else:
                val = "\n".join(block).strip()
        elif val.startswith('"') and val.endswith('"') and len(val) > 1:
            try:
                val = json.loads(val)
            except ValueError:
                val = val[1:-1]
        elif val.startswith("'") and val.endswith("'") and len(val) > 1:
            val = val[1:-1].replace("''", "'")
        out[key] = val
    return out


def _clip(text: str, limit: int) -> str:
    """At most `limit` characters, cut at a word boundary."""
    if len(text) <= limit:
        return text
    return text[: limit - 3].rsplit(" ", 1)[0].rstrip(" ,;:") + "..."


def skill_entries(meta: dict) -> list[dict]:
    """One entry per registered skill: its SKILL.md as published on main."""
    out = []
    for slug in meta:
        skill_md = registry.skill_path(slug) / "SKILL.md"
        if not skill_md.is_file():
            continue
        fields = skill_md_fields(skill_md)
        rel = skill_md.relative_to(ROOT).as_posix()
        out.append({
            "slug": slug,
            "name": fields.get("name") or slug,
            "description": _clip(" ".join(fields.get("description", "").split()), 1024),
            "url": f"{RAW_BASE}/{rel}",
            "digest": "sha256:" + hashlib.sha256(skill_md.read_bytes()).hexdigest(),
        })
    return out


def render_agent_skills(entries: list[dict]) -> str:
    """Agent Skills discovery index v0.2.0: each skill is a single SKILL.md
    (`skill-md`) whose digest is the SHA-256 of the exact bytes at `url`. The
    url is the file on GitHub (main), where its relative links still resolve;
    Jekyll would render a front-mattered SKILL.md here as an HTML page."""
    doc = {
        "$schema": AGENT_SKILLS_SCHEMA,
        "skills": [{"name": e["name"], "type": "skill-md", "description": e["description"],
                    "url": e["url"], "digest": e["digest"]} for e in sorted(entries, key=lambda e: e["name"])],
    }
    return json.dumps(doc, indent=2, ensure_ascii=False) + "\n"


def ard_entries(entries: list[dict], data: dict[str, dict]) -> list[dict]:
    """ARD entries for the same Skills, each with the questions it answers.
    They describe Skills (Markdown an agent reads), not hosted MCP endpoints:
    every MCP server here is a local program, so there is no URL to advertise
    for one."""
    items = []
    for e in entries:
        d = data.get(e["slug"], {})
        item = {
            "identifier": f"urn:air:{SITE_HOST}:skills:{e['slug']}",
            "displayName": d.get("name") or e["name"],
            "type": SKILL_MD_MEDIA_TYPE,
            "url": e["url"],
            "description": d.get("tagline") or e["description"],
        }
        if d.get("category"):
            item["tags"] = [d["category"]]
        queries = [strip_md(q) for q, _ in d.get("outcomes", []) if q][:5]
        if len(queries) >= 2:
            item["representativeQueries"] = queries
        item["metadata"] = {"page": d["link"]} if d.get("link") else {}
        if not item["metadata"]:
            del item["metadata"]
        items.append(item)
    return items


def render_ard(items: list[dict]) -> str:
    """/.well-known/ard.json: the path an ARD consumer MUST fetch (spec 5.1).
    The manifest is just the `entries` array."""
    return json.dumps({"entries": items}, indent=2, ensure_ascii=False) + "\n"


def render_ai_catalog(items: list[dict]) -> str:
    """/.well-known/ai-catalog.json: ARD's predecessor path, still what
    Lighthouse's ard-schema audit reads (its schema wants specVersion + host)."""
    doc = {
        "specVersion": "1.0",
        "host": {"displayName": "MSP Skills", "documentationUrl": SITE + "/"},
        "entries": items,
    }
    return json.dumps(doc, indent=2, ensure_ascii=False) + "\n"


# --------------------------------------------------------------------------- #
# Rendering.
# --------------------------------------------------------------------------- #
# Ordered first in "Start here" when present; the rest of the top-level pages
# follow by URL. A page that moves just falls back to that order.
START_HERE = ("/why/", "/what-is-an-mcp-server/", "/which-agent/", "/guides/first-time-setup/",
              "/install-skill/", "/install-mcp/")
SECTIONS = (
    ("Start here", lambda u: u.count("/") == 2 or u.startswith("/guides/")),
    ("Answers", lambda u: u.startswith("/answers/")),
    ("Set it up in your AI agent", lambda u: u.startswith("/integrations/")),
)


def page_link(url: str, copies: dict[str, str]) -> str:
    return SITE + copies.get(url, url)


def render_index(pages: list[dict], site: dict[str, dict], copies: dict[str, str],
                 with_install: bool = True) -> str:
    no_copy = ", ".join(SITE + u for u in sorted(set(site) - set(copies))) or "none"
    on_github = [p["name"] for p in pages if p["link"].startswith(REPO_URL)]
    github_note = (f", and {', '.join(on_github)}, which links {'its folder' if len(on_github) == 1 else 'their folders'}"
                   " on GitHub") if on_github else ""
    lines = [
        "# MSP Skills",
        "",
        "> Free MCP servers and Skills that connect your MSP tools (PSA, RMM, "
        "backup, M365, and more) to the AI you already use - Claude, ChatGPT, "
        "Copilot, Codex, Cursor, and any agent that speaks the Model Context "
        "Protocol. Local SQLite mirror so cross-client questions don't hit API "
        "rate limits. Built for MSP business owners. No code required. "
        "Apache-2.0 licensed.",
        "",
        "What this site calls an MCP server is what ChatGPT calls an app or "
        "connector, Claude on the web and Microsoft Copilot call a connector, "
        "and Claude Code calls a Skill - all the same MCP standard. Full "
        f"answer: [What is an MCP server?]({page_link('/what-is-an-mcp-server/', copies)})",
        "",
        "Links ending in `.md` are clean Markdown copies; the HTML page is the same "
        "address with a trailing slash in place of `.md`. The other links are HTML pages "
        f"with no Markdown copy ({no_copy}), the full corpus (llms-full.txt){github_note}. "
        f"Every Skill is also listed for agents at {SITE}/.well-known/agent-skills/index.json",
        "",
        "## Connectors",
        "",
    ]
    for p in pages:
        # Outcome-led: lead with the first real question the connector answers
        # (outcomes[0] from page.json) - the concrete ask is what AI search
        # engines match against; the tagline is the supporting summary.
        q0 = p["outcomes"][0][0] if p["outcomes"] else ""
        ask = f" Ask \"{q0}\"." if q0 else ""
        lines.append(f"- [{p['name']}]({p['link']}): {p['category']}.{ask} {p['tagline']} "
                     f"Verification: {p['badge']}.")
        if with_install and p["install"]:
            lines.append(f"  Install: `{p['install']}`")
    listed = {"/skills/" + p["slug"] + "/" for p in pages}
    rest = [u for u in site if u not in listed]
    for title, match in SECTIONS:
        urls = sorted((u for u in rest if u in copies and match(u)),
                      key=lambda u: (START_HERE.index(u) if u in START_HERE else len(START_HERE), u))
        if not urls:
            continue
        lines += ["", f"## {title}", ""]
        for u in urls:
            lines.append(f"- [{site[u]['title']}]({page_link(u, copies)}): {site[u]['description']}")
            rest.remove(u)
    lines += ["", "## Optional", ""]
    for u in sorted(rest):
        lines.append(f"- [{site[u]['title']}]({page_link(u, copies)}): {site[u]['description']}")
    lines.append(f"- [Full corpus for AI search]({SITE}/llms-full.txt): every connector's "
                 "outcomes, FAQ and safety summary in one file.")
    return "\n".join(lines) + "\n"


def render_full(pages: list[dict]) -> str:
    lines = [
        "# MSP Skills - full corpus",
        "",
        "> Free MCP servers and Skills connecting MSP tools to the AI you "
        "already use. Built for MSP business owners. Apache-2.0, runs locally, "
        "no data leaves your network. This file is the expanded corpus; the "
        f"short index is at {SITE}/llms.txt",
        "",
        "## What an MCP server is (for MSPs)",
        "",
        glossary_definition(),
        "",
        "One thing, many names: what this site calls an MCP server is what "
        "ChatGPT calls an app or connector, Claude on the web calls a "
        "connector, Microsoft Copilot calls a connector, and Claude Code calls "
        "a Skill. Same standard underneath: the Model Context Protocol.",
        "",
        "## Why MSP owners use msp-skills",
        "",
    ]
    for head, para in why_pillars():
        lines.append(f"### {head}")
        lines.append("")
        if para:
            lines.append(para)
            lines.append("")

    lines += ["## Connectors", ""]
    for p in pages:
        lines.append(f"### {p['name']} ({p['category']})")
        lines.append("")
        lines.append(f"Page: [{p['name']}]({p['link']})")
        lines.append(f"Verification: {p['badge']}")
        if p["install"]:
            lines.append(f"Install: {p['install']}")
        lines.append("")
        if p["intro"]:
            lines.append(p["intro"])
            lines.append("")
        if p["outcomes"]:
            lines.append("Outcomes (question -> command your AI agent runs):")
            for q, cmd in p["outcomes"]:
                if cmd:
                    lines.append(f"- {q} -> {cmd}")
                else:
                    lines.append(f"- {q}")
            lines.append("")
        if p["faqs"]:
            lines.append("FAQ:")
            for q, a in p["faqs"]:
                lines.append(f"Q: {q}")
                lines.append(f"A: {a}")
            lines.append("")
        if p["governance_summary"]:
            lines.append(f"Governance: {p['governance_summary']}")
            lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def write_if_changed(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if not path.exists() or path.read_text(encoding="utf-8") != text:
        path.write_text(text, encoding="utf-8")


def main() -> int:
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    cat_by_slug = {c["name"]: c for c in catalog.get("skills", [])}
    meta = registry.skills()  # sorted by slug

    site = site_pages()
    copies, files = build_copies(site)
    pages = [load_page_data(slug, meta[slug], cat_by_slug.get(slug, {})) for slug in meta]
    for p in pages:
        # A connector without a docs page (the markdown-only concierge) links
        # its folder on GitHub rather than a page that 404s.
        url = "/skills/" + p["slug"] + "/"
        p["link"] = page_link(url, copies) if url in site else \
            f"{REPO_URL}/tree/main/{registry.skill_path(p['slug']).relative_to(ROOT).as_posix()}"

    index = render_index(pages, site, copies)
    if len(index) > LLMS_MAX_CHARS:  # keep under the size agents read whole
        index = render_index(pages, site, copies, with_install=False)
    if len(index) > LLMS_MAX_CHARS:
        print(f"ERROR: llms.txt is {len(index):,} characters, over {LLMS_MAX_CHARS:,}", file=sys.stderr)
        return 1
    entries = skill_entries(meta)
    write_if_changed(LLMS, index)
    write_if_changed(LLMS_FULL, render_full(pages))
    write_copies(files)
    write_if_changed(AGENT_SKILLS_INDEX, render_agent_skills(entries))
    items = ard_entries(entries, {p["slug"]: p for p in pages})
    write_if_changed(ARD, render_ard(items))
    write_if_changed(AI_CATALOG, render_ai_catalog(items))
    skipped = sorted(set(site) - set(copies))
    print(f"Wrote {LLMS.relative_to(ROOT)} ({len(index):,} chars) and {LLMS_FULL.relative_to(ROOT)} "
          f"({len(pages)} connectors), {len(files)} Markdown copies "
          f"(no copy: {', '.join(skipped) or 'none'}), {len(entries)} Skills in "
          f"{AGENT_SKILLS_INDEX.relative_to(ROOT)}, {ARD.relative_to(ROOT)} and {AI_CATALOG.relative_to(ROOT)}.")
    return 0


def self_test() -> int:
    """Assert-based checks of the helpers and of what main() would write."""
    assert page_url(Path("skills/acronis.md"), {}) == "/skills/acronis/"
    assert page_url(Path("index.md"), {}) == "/"
    assert page_url(Path("x.md"), {"permalink": "/y/"}) == "/y/"
    assert copy_path("/") == "/index.md" and copy_path("/skills/acronis/") == "/skills/acronis.md"
    assert plain_text("a \u2014 b") == "a - b", plain_text("a \u2014 b")
    assert html_to_markdown('<a href="/x/">X</a> <code>y</code>&nbsp;z') == "[X](/x/) `y` z"
    assert html_to_markdown("<code>a --org &lt;id&gt;</code>") == "`a --org <id>`"
    assert _prose_to_markdown("`<a>` [t](/x/){: .btn}", DOCS / "x.md", lambda t, s: "ABS" + t) \
        == "`<a>` [t](ABS/x/)"
    tmp = ROOT / ".build-llms-selftest.md"
    tmp.write_text("---\nname: demo\ndescription: >\n  one\n  two\nlicense: x\n---\n", encoding="utf-8")
    try:
        assert skill_md_fields(tmp) == {"name": "demo", "description": "one two"}
    finally:
        tmp.unlink()

    site = site_pages()
    copies, files = build_copies(site)
    assert "/skills/acronis/" in copies and copies["/install-skill/"] == "/install-skill.md"
    for path, text in files.items():
        assert text.startswith("> For AI agents:"), path
        assert "{%" not in text and "{{" not in text, path
        for target in re.findall(r"\]\(([^)\s]+)", text):
            assert target.startswith(("https://", "http://", "mailto:")), (path, target)
    resolve = LinkResolver(copies, site)
    assert resolve("/why/", DOCS / "x.md") == SITE + "/why.md"
    assert resolve("#install", DOCS / "skills" / "acronis.md") == SITE + "/skills/acronis/#install"

    meta = registry.skills()
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    cat_by_slug = {c["name"]: c for c in catalog.get("skills", [])}
    pages = [load_page_data(s, meta[s], cat_by_slug.get(s, {})) for s in meta]
    for p in pages:
        p["link"] = page_link("/skills/" + p["slug"] + "/", copies)
    index = render_index(pages, site, copies)
    links = re.findall(r"\[[^\]]+\]\((https://[^)]+)\)", index)
    assert index.startswith("# ") and len(links) >= len(pages), len(links)
    assert len(index) <= LLMS_MAX_CHARS or len(render_index(pages, site, copies, False)) <= LLMS_MAX_CHARS
    for link in links:
        if link.startswith(SITE) and link.endswith(".md"):
            assert link[len(SITE):] in files, link
    entries = skill_entries(meta)
    assert len(entries) == len(meta)
    for e in entries:
        rel = e["url"][len(RAW_BASE) + 1:]
        assert e["digest"] == "sha256:" + hashlib.sha256((ROOT / rel).read_bytes()).hexdigest()
        assert 0 < len(e["description"]) <= 1024 and re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", e["name"])
    for p in pages:  # main() links a connector with no docs page to GitHub
        if "/skills/" + p["slug"] + "/" not in site:
            p["link"] = REPO_URL + "/tree/main/x"
    head = render_index(pages, site, copies).split("## Connectors")[0]
    assert "Links ending in `.md`" in head and SITE + "/skills/" in head, head
    items = ard_entries(entries, {p["slug"]: p for p in pages})
    ard = json.loads(render_ard(items))
    assert list(ard) == ["entries"] and len(ard["entries"]) == len(entries)
    for item in ard["entries"]:  # ARD 4.2: these three, and exactly one of url / data
        assert item["identifier"].startswith("urn:air:") and item["displayName"] and item["type"]
        assert ("url" in item) != ("data" in item), item["identifier"]
    assert json.loads(render_ai_catalog(items))["entries"] == ard["entries"]
    print(f"self-test OK: {len(files)} copies, {len(links)} links in llms.txt, {len(entries)} skills")
    return 0


if __name__ == "__main__":
    sys.exit(self_test() if "--self-test" in sys.argv[1:] else main())
