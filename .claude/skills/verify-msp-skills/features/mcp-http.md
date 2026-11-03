# MCP HTTP transport

Remote agents need `<slug>-mcp --transport http`. Started without a bearer
token, the server must refuse to start or listen on loopback only; started
with `<PREFIX>_MCP_HTTP_TOKEN`, it must answer only requests carrying
`Authorization: Bearer <token>`. From press 4.34.2 on it also refuses a browser
`Origin` that is not loopback or listed in `<PREFIX>_MCP_HTTP_ALLOWED_ORIGINS`.

## Sub-features

- `http-no-token-default` is the documented launch with no token variable: refuse to start, or bind 127.0.0.1 only.
- `http-bearer-required` answers 401 (or 403) to an MCP request without the token, and to one with a wrong token.
- `http-bearer-works` answers `initialize` and `tools/list` with the token and no `Origin` header.
- `http-origin-allowlist` answers 403 to a request with the token and a foreign `Origin`. Expected from press
  4.34.2 on (the recorded press version, or a binary that reads `<PREFIX>_MCP_HTTP_ALLOWED_ORIGINS`); on an
  older build that answers it, the check is SKIP, never FAIL.

## How to get to it (user POV)

- `<PREFIX>_MCP_HTTP_TOKEN=<token> <slug>-mcp --transport http` (default address).
- `<slug>-mcp --transport http --addr 127.0.0.1:<port>` for a local reverse proxy or tunnel.
- `PP_MCP_TRANSPORT=http` in the environment instead of the flag.

## Driving it with verify.py

Preconditions:

- `verify.py doctor --slug <slug>` reports the default HTTP port free and a socket reader (`lsof` or `ss`).
- No `<PREFIX>_MCP_HTTP_TOKEN` in the driven environment (it is built from nothing).

- **Default launch, no token.** Run `verify.py run --slug <slug> --features 3`.
  The harness reads the default address from `<slug>-mcp --help`, takes the
  default-port lock, starts `<slug>-mcp --transport http`, and for up to ten
  seconds watches for exit or a listening socket (`lsof -nP -a -p <pid>
  -iTCP -sTCP:LISTEN`). `f3-mcp-http/default-launch.json` records the
  listening addresses or the exit code and stderr.
- **Judge the bind.** An exit that names the token variable is PASS; listening
  only on 127.0.0.1 or ::1 is PASS; listening on `*` or `0.0.0.0` is FAIL with
  the address.
- **Bearer auth.** The harness starts the server again with a random 64-hex
  token and `--addr 127.0.0.1:<free port>`, POSTs `initialize` to `/mcp`
  with no `Authorization` header, with a different random token, with the
  token and `Origin: https://verify-foreign-origin.invalid`, and with the token
  and no `Origin`, then `notifications/initialized` and `tools/list` with the
  `Mcp-Session-Id`. `bearer-auth.json` records each HTTP status, the tool
  count, and why an Origin allowlist was or was not expected.
- **Proof.** `bearer auth` PASS reads `no token: HTTP 401; wrong token: HTTP 401;
  Bearer $<PREFIX>_MCP_HTTP_TOKEN: HTTP 200 and <N> tools`. `origin allowlist`
  PASS reads `foreign Origin with the token: HTTP 403; no Origin with the
  token: HTTP 200`; before press 4.34.2 it is SKIP with the status observed.
- **Self-test.** `verify.py self-test` drives the same probe against stand-in
  servers (one that enforces both, one with no allowlist, one that accepts any
  bearer, one that ignores `Origin` although it should not, one that refuses a
  request with no `Origin`, one with no auth) and checks every verdict.

## Gotchas

- On a build that binds every interface with no token, the check briefly runs
  such a server (inside the sandbox, with stand-in credentials and the fake
  vendor) to observe it, then stops it within seconds.
- The default port is shared by every run on the machine; a port already in
  use is ERROR, never a guess about the bind.
- The token, and the wrong token, are redacted from every evidence file.
- Older builds ignore the token variable, so the unauthenticated request gets
  HTTP 200: that is the recorded FAIL.
- The static rule `mcp-http-no-bearer-auth` in `check_security_gate.py` reads
  the source; this feature observes the running server.
