#!/usr/bin/env python3
"""fake_github.py - a stand-in for api.github.com AND github.com on one port.

The installers point at it through MSP_SKILLS_API_BASE (which relocates both
origins). Routes served:

  GET /repos/<owner>/<repo>/releases?per_page=100&page=N
        page 1: 100 unrelated skills' tags (so pagination is exercised);
        page 2: the fixture tag among unrelated tags; page 3+: [].
        --list-status N replies N instead.
  GET /repos/<owner>/<repo>/releases/tags/<tag>
        one release object; --immutable controls its "immutable" field:
        true | false | missing | string | null | nested (true at top level AND
        a nested object carrying false) | nested-only (NO top-level field, a
        nested object carrying true) | broken (top-level value is the malformed
        token trueBROKEN, i.e. invalid JSON). --tag-status N replies N instead.
  GET /<owner>/<repo>/releases/download/<tag>/<asset>
        the file <assets>/<asset>, 404 when absent. --truncate ASSET announces
        the real Content-Length but sends only a proper prefix of the bytes.

Every request line is appended to --log as "METHOD path status auth=0|1",
auth=1 meaning an Authorization header arrived. The chosen port is written to
--port-file once the socket is bound. Stdlib only.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

A = None  # parsed args


def release_object() -> dict:
    # The body deliberately carries an ESCAPED copy of the field with the
    # opposite value, to prove the installers read the field, not the prose.
    opposite = "false" if A.immutable == "true" else "true"
    obj = {
        "tag_name": A.tag,
        "name": A.tag,
        "draft": False,
        "prerelease": False,
        "body": f'Release notes. See "immutable": {opposite} discussed in the docs.',
        "author": {"login": "github-actions[bot]"},
        "assets": [{"name": n, "uploader": {"login": "github-actions[bot]"}}
                   for n in sorted(os.listdir(A.assets))],
    }
    if A.immutable == "true":
        obj["immutable"] = True
    elif A.immutable == "false":
        obj["immutable"] = False
    elif A.immutable == "string":
        obj["immutable"] = "true"
    elif A.immutable == "null":
        obj["immutable"] = None
    elif A.immutable == "nested":
        obj["immutable"] = True
        obj["source"] = {"immutable": False}
    elif A.immutable == "nested-only":
        obj["source"] = {"immutable": True}
    elif A.immutable == "broken":
        obj["immutable"] = True  # rewritten to trueBROKEN in the serializer
    # "missing": no field at all
    return obj


def release_bytes() -> bytes:
    text = json.dumps(release_object(), indent=2)
    if A.immutable == "broken":
        text = text.replace('"immutable": true', '"immutable": trueBROKEN', 1)
    return text.encode()


class H(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):  # quiet stderr; --log gets the lines
        pass

    def _log(self, status: int) -> None:
        if A.log:
            auth = 1 if self.headers.get("Authorization") else 0
            with open(A.log, "a", encoding="utf-8") as f:
                f.write(f"{self.command} {self.path} {status} auth={auth}\n")

    def _send(self, status: int, body: bytes, ctype: str = "application/json") -> None:
        self._log(status)
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        u = urlparse(self.path)
        q = parse_qs(u.query)
        api_prefix = f"/repos/{A.owner}/{A.repo}/releases"
        dl_prefix = f"/{A.owner}/{A.repo}/releases/download/{A.tag}/"

        if u.path == api_prefix:
            if A.list_status:
                return self._send(A.list_status, b'{"message":"boom"}')
            page = int(q.get("page", ["1"])[0])
            if page == 1:
                body = [{"tag_name": f"other{i}-v1.0.{i}"} for i in range(100)]
            elif page == 2:
                body = [{"tag_name": "acronis-v0.1.2"}, {"tag_name": A.tag},
                        {"tag_name": f"{A.slug}-v0.0.1"}, {"tag_name": "zammad-v0.1.3"}]
            else:
                body = []
            return self._send(200, json.dumps(body, indent=2).encode())

        if u.path == f"{api_prefix}/tags/{A.tag}":
            if A.tag_status:
                return self._send(A.tag_status, b'{"message":"boom"}')
            return self._send(200, release_bytes())

        if u.path.startswith(dl_prefix):
            name = u.path[len(dl_prefix):]
            p = os.path.join(A.assets, name)
            if "/" in name or not os.path.isfile(p):
                return self._send(404, b"Not Found", "text/plain")
            with open(p, "rb") as f:
                data = f.read()
            if A.truncate == name:
                # Announce the true length, send a proper prefix, close.
                self._log(200)
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data[: max(1, len(data) - 8)])
                self.wfile.flush()
                self.close_connection = True
                return None
            return self._send(200, data, "application/octet-stream")

        return self._send(404, b"Not Found", "text/plain")


def main() -> int:
    global A
    ap = argparse.ArgumentParser()
    ap.add_argument("--port-file", required=True)
    ap.add_argument("--assets", required=True)
    ap.add_argument("--slug", required=True)
    ap.add_argument("--tag", required=True)
    ap.add_argument("--owner", default="servosity")
    ap.add_argument("--repo", default="msp-skills")
    ap.add_argument("--immutable", default="true",
                    choices=["true", "false", "missing", "string", "null", "nested", "nested-only", "broken"])
    ap.add_argument("--list-status", type=int, default=0)
    ap.add_argument("--tag-status", type=int, default=0)
    ap.add_argument("--truncate", default="")
    ap.add_argument("--log", default="")
    A = ap.parse_args()
    srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
    with open(A.port_file + ".tmp", "w", encoding="utf-8") as f:
        f.write(str(srv.server_address[1]))
    os.replace(A.port_file + ".tmp", A.port_file)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
