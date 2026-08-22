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

# Line continuations: bash `\`, cmd `^`, PowerShell backtick - each at EOL.
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


_FLAGS = ("--header", "--cookie", "-H", "-b")


def _iter_flag_values(text: str):
    """Yield (flag, unescaped_value) for each -H/--header/-b/--cookie in SOURCE ORDER.

    Walks the blob once, honouring quote state, so a quoted value is consumed
    whole and its contents can never be re-parsed as another flag. Handles the
    three quote styles Chrome emits ($'...', '...', "...") plus an unquoted
    token, and skips over any other quoted run so flags inside it stay inert.
    """
    i, n = 0, len(text)
    while i < n:
        c = text[i]
        # Skip a quoted run that is not preceded by one of our flags: whatever
        # is inside belongs to that value, not to the command line.
        if c == "'":
            j = i + 1
            while j < n and text[j] != "'":
                j += 1
            i = j + 1
            continue
        if c == '"':
            j = i + 1
            while j < n:
                if text[j] == "\\":
                    j += 2
                    continue
                if text[j] == '"':
                    break
                j += 1
            i = j + 1
            continue
        matched = None
        for flag in _FLAGS:
            if text.startswith(flag, i):
                after = i + len(flag)
                # Require a real boundary so --headerish does not match -H.
                if after < n and (text[after].isspace() or text[after] in "'\"$"):
                    matched = (flag, after)
                    break
        if matched is None:
            i += 1
            continue
        flag, j = matched
        while j < n and text[j].isspace():
            j += 1
        if j >= n:
            break
        if text.startswith("$'", j):  # ANSI-C
            k = j + 2
            buf = []
            while k < n and text[k] != "'":
                if text[k] == "\\" and k + 1 < n:
                    buf.append(text[k:k + 2])
                    k += 2
                    continue
                buf.append(text[k])
                k += 1
            yield flag, _unescape_ansic("".join(buf))
            i = k + 1
        elif text[j] == "'":  # plain single: no escapes inside
            k = text.find("'", j + 1)
            if k == -1:
                break
            yield flag, text[j + 1:k]
            i = k + 1
        elif text[j] == '"':  # double, backslash escapes
            k = j + 1
            buf = []
            while k < n:
                if text[k] == "\\" and k + 1 < n:
                    buf.append(text[k:k + 2])
                    k += 2
                    continue
                if text[k] == '"':
                    break
                buf.append(text[k])
                k += 1
            yield flag, _unescape_double("".join(buf))
            i = k + 1
        else:  # bare token
            k = j
            while k < n and not text[k].isspace():
                k += 1
            yield flag, text[j:k]
            i = k


def parse_headers(text: str) -> dict[str, str]:
    """Parse a Copy-as-cURL blob into a lowercased header map.

    The raw cookie (`-b`/`--cookie` or a `cookie:` header) is always available
    under the `cookie` key. Later occurrences overwrite earlier ones.
    """
    text = _CONT.sub(" ", text)
    headers: dict[str, str] = {}
    cookie_flag: str | None = None

    # Scan ONCE, in source order, tracking quote state -- never three separate
    # passes over the whole blob. Three passes are quoting-unaware: text that
    # merely LOOKS like `-H "..."` inside somebody else's quoted value is picked
    # up as a real header, and because the double-quote pass ran last it also
    # WINS. That lets any attacker-controlled substring of the victim's own
    # request (a cookie value, a URL query parameter) choose which credential
    # gets stored and wired -- and verify still passes, because the injected
    # session is live. Source order is also what the docstring promises.
    for flag, value in _iter_flag_values(text):
        if flag in ("-H", "--header"):
            _add_header(headers, value)
        elif flag in ("-b", "--cookie"):
            # -b / --cookie takes precedence for the cookie value if present.
            cookie_flag = value

    if cookie_flag is not None:
        headers["cookie"] = cookie_flag

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
