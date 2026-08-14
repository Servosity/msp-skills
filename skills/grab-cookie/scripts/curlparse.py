# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Robust Copy-as-cURL header/cookie parser.

A naive parser handles only bash-style `-H '...'` and breaks on the rest.
Chrome's "Copy as cURL" has two flavours that differ in quoting and line
continuation, and which one a user gets depends on their platform:

  - Copy as cURL (bash) : -H 'name: value'   , backslash line continuation
  - Copy as cURL (cmd)  : -H "name: value"   , caret (^) line continuation

A backtick line continuation is also accepted, since a curl command reflowed in
a PowerShell buffer uses one. We also see bash ANSI-C quoting `-H $'name: value'`
when a value carries bytes that need escaping. This module handles all of that
and returns a lowercased header map (with the cookie merged under `cookie`).
Values are never printed; this module only parses text.

NOT SUPPORTED -- Chrome's "Copy as PowerShell" MENU ITEM. That is a different
thing from a curl command with backticks in it: it emits `Invoke-WebRequest`
with the headers in a PowerShell hashtable (`-Headers @{...}`), so there is no
`-H` or `--header` token anywhere to find. Pasting it parses to an empty header
map, and the failure surfaces later as the misleading "required credential not
found". Choose "Copy as cURL (bash)".

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

    # 8. backtick continuation (a curl command reflowed in a PowerShell buffer;
    #    NOT Chrome's "Copy as PowerShell", which emits Invoke-WebRequest)
    h = parse_headers('curl "https://x" `\n  -H "x-realm: backtick"')
    assert h["x-realm"] == "backtick", h

    # 9. Chrome's "Copy as PowerShell" carries no -H tokens at all, so it must
    #    parse to nothing rather than appearing to half-work.
    pwsh = ('$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession\n'
            'Invoke-WebRequest -UseBasicParsing -Uri "https://x" `\n'
            '  -Headers @{"x-realm"="nope"; "authorization"="Bearer zzz"}')
    assert parse_headers(pwsh) == {}, "Invoke-WebRequest must not parse as curl"

    print("curlparse.py selfcheck OK (bash/cmd/backtick quoting, ANSI-C, -b + cookie header)")


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
