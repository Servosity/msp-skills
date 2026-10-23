# MCP HTTP transport

Remote agents need `<slug>-mcp --transport http`. Started without a bearer
token, the server must refuse to start or listen on loopback only; started
with `<PREFIX>_MCP_HTTP_TOKEN`, it must answer only requests carrying
`Authorization: Bearer <token>`.

## Sub-features

- `http-no-token-default` is the documented launch with no token variable: refuse to start, or bind 127.0.0.1 only.
- `http-bearer-required` answers 401 (or 403) to an MCP request without the token.
- `http-bearer-works` answers `initialize` and `tools/list` with the token.

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
  without and then with `Authorization: Bearer <token>`, then
  `notifications/initialized` and `tools/list` with the `Mcp-Session-Id`.
  `bearer-auth.json` records each HTTP status and the tool count.
- **Proof.** PASS reads `no token: HTTP 401; Bearer $<PREFIX>_MCP_HTTP_TOKEN: HTTP 200 and <N> tools`.

## Gotchas

- On a build that binds every interface with no token, the check briefly runs
  such a server (inside the sandbox, with stand-in credentials and the fake
  vendor) to observe it, then stops it within seconds.
- The default port is shared by every run on the machine; a port already in
  use is ERROR, never a guess about the bind.
- The token is redacted from every evidence file.
- Older builds ignore the token variable, so the unauthenticated request gets
  HTTP 200: that is the recorded FAIL.
- The static rule `mcp-http-no-bearer-auth` in `check_security_gate.py` reads
  the source; this feature observes the running server.
