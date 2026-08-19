# OpenCLI browser crib + credential-store conventions

## Binding the user's real Chrome

```bash
opencli doctor                          # want [OK] Daemon + [OK] Extension connected
opencli browser <slug> bind             # bind the CURRENTLY FOCUSED Chrome tab/window
opencli browser <slug> state            # URL + title + indexed [N] interactive elements
opencli browser <slug> unbind           # detach WITHOUT closing the tab (end of run)
```

Use a per-target session `<slug>` (e.g. `halopsa`, `ninjaone`). Drive `--window foreground`
so the user watches live. `scripts/preflight.py` does doctor+bind+state and refuses to run
if the bridge is down (it never falls back to a spawned browser).

**What `bind` actually does, and what to tell the user (issue #266).** The bridge extension
does not always drive the tab you had focused. It manages its own "owned container" windows
and reuses a persistent `about:blank` placeholder, so a **new, apparently blank Chrome window
can appear** instead of your existing tab. Running `opencli doctor` can create one by itself.
Upstream: <https://github.com/jackwener/opencli/issues/2202> (open; observed on macOS, so this
is the bridge's design, not a Windows problem).

This is cosmetically alarming and easy to read as "it opened a browser I am not logged into,"
which is the exact fear this skill exists to avoid. When the window really is OpenCLI's own
container, it is not that: the extension creates it **inside your existing Chrome profile**,
which is where your cookies and sessions live, so it carries your logins.

**Confirm that before you rely on it.** A blank window is not evidence of anything - it does not
tell you which profile you are driving. `open` the vendor page and read `state`: you want an
authenticated URL and the user's own identity on the page, not a login form. Once that passes,
say out loud that the new window is their session rather than letting them assume the run went
wrong. **Until it passes, treat the window exactly like the *separate browser* the SKILL.md
guardrail forbids** (Playwright, Puppeteer, a headless Chromium): no secret capture, no secret
entry, no `guard_click` on a consent screen.

If the check fails, or the container window stays blank and never navigates, in order:
1. Reload the OpenCLI card in `chrome://extensions/` (dormant MV3 worker; a daemon restart
   does not wake it).
2. Focus the real tab and `opencli browser <slug> bind` again.
3. Last resort, the `--remote-debugging-port=9222` CDP path in `references/opencli-bootstrap.md`.

## Navigation / interaction (non-secret values only)

```bash
opencli browser <slug> open  "<url>"
opencli browser <slug> state                         # re-read indexed elements after each change
opencli browser <slug> find  --selector "<css|text>" # {matches_n, entries[]}
opencli browser <slug> click "<selector|N>"          # prefer guard_click.py for risky clicks
opencli browser <slug> fill  "<selector>" "<value>"  # verifies the value landed
opencli browser <slug> select "<selector>" "<option>"
opencli browser <slug> check "<selector>"            # checkbox/radio
opencli browser <slug> upload "<selector>" /path/file.png   # native file picker
opencli browser <slug> wait  selector "<css>"        # also: text / time / xhr / download
opencli browser <slug> screenshot "$RUN/NN-step.png" # NON-secret pages only
opencli browser <slug> extract                       # page text as markdown (nav discovery)
```

**Never** for secrets: `eval` a secret node yourself, `network`, `console`, `get url`, a
clipboard read, or a screenshot of a revealed value. Capture via
`grab_secret.py`/`oauth_login.py`.

### The clipboard gotcha (verified)

OpenCLI's page **"Copy to clipboard" button does not reliably reach the system clipboard**;
a clipboard read returns stale or unrelated content (this once captured an unrelated note
and nearly stored it as a credential). Always read the DOM `innerText`/`value` via the
helper, never click-Copy and then read the clipboard.

## Credential store - no-echo write/read

Never call the platform store by hand. `scripts/credstore.py` is the one interface, and it
picks the right backend (macOS Keychain, Windows Credential Manager) and prints only a
redacted receipt:

```
uv run scripts/grab_secret.py --session <slug> --selector '<css>' --service <SVC> --account <acct>
uv run scripts/credstore.py --store --service <SVC> --account <acct>   # Lane C, hidden prompt
```

Reading a credential back is the launcher's job, not yours:
`scripts/mint_wrapper.py` stamps a launcher that fetches it in-process at exec time.

Confirm only with `len` / `sha256[:8]` / `last4`, never the value.

**Naming:** `account = <vendor slug>` (e.g. `halopsa`), `service = <VENDOR>_<ARTIFACT>` (e.g.
`HALOPSA_API_KEY`, `NINJAONE_REFRESH_TOKEN`). On Windows these become the Credential Manager
target `Servosity/connect-tool/<service>/<account>`.

## Wiring the consumer

- **CLI:** `uv run scripts/mint_wrapper.py <name> <ENV_VAR> <account> <SERVICE> <absolute-binary>`
  stamps a launcher (`~/.local/bin/<name>` on macOS, `<app-dir>\bin\<name>.cmd` on Windows)
  that reads the secret from the credential store at launch. The launcher itself holds no
  credential logic; it calls back into Python, which fetches in-process.
- **MCP:** point the MCP command at that launcher, so the value never lands in the MCP
  config. Avoid `--env KEY="$(...)"`, which writes the resolved value into the config file.
  On Windows, prefer an absolute `uv.exe` plus the script path over a `.cmd` or `.ps1`,
  which are not reliably spawnable without a shell. Restart Claude Code after `mcp add`.
