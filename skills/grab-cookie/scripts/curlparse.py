# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Robust Copy-as-cURL header/cookie parser.

A naive parser handles only bash-style `-H '...'` and breaks on the others.
Chrome on Windows offers three "Copy as cURL" flavours and they differ in
quoting and line continuation, so which one a user pastes depends on their
platform and on which DevTools build they are in:

  - Copy as cURL (bash) : -H 'name: value'   , backslash line continuation
  - Copy as cURL (cmd)  : -H "name: value"   , caret (^) line continuation
  - PowerShell          : backtick line continuation, backtick-quote escaping

We also see bash ANSI-C quoting `-H $'name: value'` when a value carries bytes
that need escaping. This module handles all of them and returns a lowercased
header map (with the cookie merged under `cookie`). Values are never printed;
this module only parses text.

  parse_headers_from_file(path) -> dict[str, str]
  parse_headers(text)          -> dict[str, str]
  python curlparse.py --selfcheck
"""
from __future__ import annotations

import re
import sys

# -H / --header, in the three quote styles. Order: ANSI-C first ($'...') so the
# leading `$` is consumed, then plain single, then double.
_HDR_ANSIC = re.compile(r"(?:-H|--header)\s+\$'((?:[^'\\]|\\.)*)'")
_HDR_SINGLE = re.compile(r"(?:-H|--header)\s+'([^']*)'")
_HDR_DOUBLE = re.compile(r'(?:-H|--header)\s+"((?:[^"\\]|\\.)*)"')

# -b / --cookie, same three styles.
_CK_ANSIC = re.compile(r"(?:-b|--cookie)\s+\$'((?:[^'\\]|\\.)*)'")
_CK_SINGLE = re.compile(r"(?:-b|--cookie)\s+'([^']*)'")
_CK_DOUBLE = re.compile(r'(?:-b|--cookie)\s+"((?:[^"\\]|\\.)*)"')

# Line continuations: bash `\`, cmd `^`, PowerShell backtick — each at EOL.
_CONT = re.compile(r"[\\^`]\r?\n")


def _unescape_double(s: str) -> str:
    """Undo bash/cmd double-quote backslash escaping (\\", \\\\, \\`, \\$)."""
    out = []
    i = 0
    while i < len(s):
        c = s[i]
        if c == "\\" and i + 1 < len(s) and s[i + 1] in '"\\`$':
            out.append(s[i + 1])
            i += 2
        else:
            out.append(c)
            i += 1
    return "".join(out)


def _unescape_ansic(s: str) -> str:
    """Undo bash ANSI-C ($'...') escaping for the sequences Chrome emits."""
    simple = {"n": "\n", "r": "\r", "t": "\t", "\\": "\\", "'": "'", '"': '"', "0": "\0"}
    out = []
    i = 0
    while i < len(s):
        c = s[i]
        if c != "\\" or i + 1 >= len(s):
            out.append(c)
            i += 1
            continue
        nxt = s[i + 1]
        if nxt == "x" and i + 3 < len(s) + 1:
            hexpart = s[i + 2 : i + 4]
            try:
                out.append(chr(int(hexpart, 16)))
                i += 4
                continue
            except ValueError:
                pass
        if nxt == "u" and i + 5 < len(s) + 1:
            hexpart = s[i + 2 : i + 6]
            try:
                out.append(chr(int(hexpart, 16)))
                i += 6
                continue
            except ValueError:
                pass
        if nxt in simple:
            out.append(simple[nxt])
            i += 2
            continue
        out.append(nxt)  # unknown escape: drop the backslash, keep the char
        i += 2
    return "".join(out)


def _add_header(headers: dict[str, str], raw: str) -> None:
    """Split one `Name: value` header string and store it lowercased."""
    idx = raw.find(":")
    if idx <= 0:
        return
    name = raw[:idx].strip().lower()
    value = raw[idx + 1 :].strip()
    if name:
        headers[name] = value


def parse_headers(text: str) -> dict[str, str]:
    """Parse a Copy-as-cURL blob into a lowercased header map.

    The raw cookie (`-b`/`--cookie` or a `cookie:` header) is always available
    under the `cookie` key. Later occurrences overwrite earlier ones.
    """
    text = _CONT.sub(" ", text)
    headers: dict[str, str] = {}

    for m in _HDR_ANSIC.finditer(text):
        _add_header(headers, _unescape_ansic(m.group(1)))
    for m in _HDR_SINGLE.finditer(text):
        _add_header(headers, m.group(1))
    for m in _HDR_DOUBLE.finditer(text):
        _add_header(headers, _unescape_double(m.group(1)))

    # -b / --cookie takes precedence for the cookie value if present.
    for rx, unesc in ((_CK_ANSIC, _unescape_ansic), (_CK_SINGLE, None), (_CK_DOUBLE, _unescape_double)):
        m = rx.search(text)
        if m:
            headers["cookie"] = unesc(m.group(1)) if unesc else m.group(1)

    return headers


def parse_headers_from_file(path: str) -> dict[str, str]:
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        return parse_headers(fh.read())


def _selfcheck() -> None:
    # 1. bash single-quote
    h = parse_headers("curl 'https://x' -H 'authorization: tok123' -H 'x-realm: rrr'")
    assert h["authorization"] == "tok123" and h["x-realm"] == "rrr", h

    # 2. bash double-quote with an escaped quote in the value
    h = parse_headers('curl "https://x" -H "authorization: a\\"b"')
    assert h["authorization"] == 'a"b', h

    # 3. ANSI-C quoting with a hex escape
    h = parse_headers(r"curl 'https://x' -H $'x-test: a\x2Db'")
    assert h["x-test"] == "a-b", h

    # 4. cmd-style `^` line continuation + double quotes
    cmd = 'curl "https://x" ^\n  -H "accept: application/json" ^\n  -H "cookie: example_session=s%3Aabc; _ga=1"'
    h = parse_headers(cmd)
    assert h["accept"] == "application/json", h
    assert h["cookie"].startswith("example_session=s%3Aabc"), h

    # 5. cookie via -b flag (not a header)
    h = parse_headers("curl 'https://x' -b 'example_session=s%3Axyz; other=2'")
    assert h["cookie"].startswith("example_session=s%3Axyz"), h

    # 6. cookie via -H 'cookie: ...'
    h = parse_headers("curl 'https://x' -H 'cookie: example_session=s%3Aqqq'")
    assert h["cookie"] == "example_session=s%3Aqqq", h

    # 7. --header long form + value containing a colon (split on first only)
    h = parse_headers("curl 'https://x' --header 'authorization: Bearer a:b:c'")
    assert h["authorization"] == "Bearer a:b:c", h

    # 8. PowerShell backtick continuation
    h = parse_headers('curl "https://x" `\n  -H "x-realm: pwsh"')
    assert h["x-realm"] == "pwsh", h

    print("curlparse.py selfcheck OK (bash/cmd/pwsh quoting, ANSI-C, -b + cookie header)")


if __name__ == "__main__":
    if "--selfcheck" in sys.argv:
        _selfcheck()
        raise SystemExit(0)
    if len(sys.argv) > 1:
        # Debug aid: print only header NAMES found in a capture file (never values).
        hdrs = parse_headers_from_file(sys.argv[1])
        print("headers found:", ", ".join(sorted(hdrs.keys())) or "(none)")
    else:
        print(__doc__)
