# OpenCLI browser crib + Keychain conventions

## Binding the user's real Chrome

```bash
opencli doctor                          # want [OK] Daemon + [OK] Extension connected
opencli browser <slug> bind             # bind the CURRENTLY FOCUSED Chrome tab/window
opencli browser <slug> state            # URL + title + indexed [N] interactive elements
opencli browser <slug> unbind           # detach WITHOUT closing the tab (end of run)
```

Use a per-target session `<slug>` (e.g. `halopsa`, `ninjaone`). Drive `--window foreground`
so the user watches live. `scripts/preflight.sh` does doctor+bind+state and refuses to run
if the bridge is down (it never falls back to a spawned browser).

## Navigation / interaction (non-secret values only)

```bash
opencli browser <slug> open  "<url>"
opencli browser <slug> state                         # re-read indexed elements after each change
opencli browser <slug> find  --selector "<css|text>" # {matches_n, entries[]}
opencli browser <slug> click "<selector|N>"          # prefer guard_click.sh for risky clicks
opencli browser <slug> fill  "<selector>" "<value>"  # verifies the value landed
opencli browser <slug> select "<selector>" "<option>"
opencli browser <slug> check "<selector>"            # checkbox/radio
opencli browser <slug> upload "<selector>" /path/file.png   # native file picker
opencli browser <slug> wait  selector "<css>"        # also: text / time / xhr / download
opencli browser <slug> screenshot "$RUN/NN-step.png" # NON-secret pages only
opencli browser <slug> extract                       # page text as markdown (nav discovery)
```

**Never** for secrets: `eval` a secret node yourself, `network`, `console`, `get url`,
`pbpaste`, or a screenshot of a revealed value. Capture via `grab_secret.sh`/`oauth_login.sh`.

### The pbpaste gotcha (verified)

OpenCLI's page **"Copy to clipboard" button does not reliably reach the macOS pasteboard**;
`pbpaste` returns stale/unrelated content (once captured an unrelated voice memo and nearly
stored it as a credential). Always read the DOM `innerText`/`value` via the helper - never
click-Copy + `pbpaste`.

## Keychain - no-echo write/read

Write (value set inside the shell, never printed). `-U` updates if present:
```bash
security add-generic-password -U -a "<account>" -s "<SERVICE>" -w "$value"
```
Read straight into a variable (never to a bare terminal):
```bash
export VAR="$(security find-generic-password -a <account> -s <SERVICE> -w 2>/dev/null)"
```
Interactive (Lane C - hidden prompt, no value in argv):
```bash
security add-generic-password -U -a <account> -s <SERVICE> -w
```
Confirm only with `len` / `sha256[:8]` / `last4` - never the value.

**Naming:** `account = <vendor slug>` (e.g. `halopsa`), `service = <VENDOR>_<ARTIFACT>` (e.g.
`HALOPSA_API_KEY`, `NINJAONE_REFRESH_TOKEN`).

## Wiring the consumer

- **CLI:** `scripts/mint_wrapper.sh <name> <ENV_VAR> <account> <SERVICE> <real-binary>` stamps
  `~/.local/bin/<name>` that reads the secret from the Keychain at launch (value never in a file).
- **MCP:** `claude mcp add <name> -- "/path/to/keychain-wrapper"` - point the command at a
  wrapper that reads the Keychain, so the value never lands in the MCP config. (Avoid
  `--env KEY="$(security …)"`, which writes the resolved value into the config file.) Restart
  Claude Code after `mcp add`.
