# OpenCLI bootstrap - install + configure when missing or disconnected

`scripts/preflight.py` runs `opencli doctor` and refuses to proceed unless it shows
`[OK] Daemon` **and** `[OK] Extension`. When it doesn't, offer to set it up. Never fall
back to a spawned browser (Playwright, Puppeteer, headless Chromium): it carries no login.

## Decision tree

1. **`opencli` binary missing** → offer to install. It needs **Node 20 or newer**:
   ```
   npm install -g @jackwener/opencli
   ```
   If Node is missing: `brew install node` (macOS) or `winget install OpenJS.NodeJS.LTS`
   (Windows). `preflight.py --deps` checks the Node major version, not just its presence.
2. **Daemon not running** (`opencli doctor` has no `[OK] Daemon`) → the daemon auto-starts on
   first `opencli browser …` use; if it's wedged: `opencli daemon restart`.
3. **Extension not connected** (`opencli doctor` reports `[MISSING] Extension` or anything
   other than `[OK] Extension`) → install it from the **Chrome Web Store**, one click, no
   developer mode, same on macOS and Windows:

   <https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk>

   - Click **Add to Chrome**, accept the permission prompt, and keep Chrome open.
   - Re-run `opencli doctor` and **loop until `[OK] Extension: connected`** before any nav.
   - Only if the Web Store is blocked by policy: download
     `opencli-extension-v<version>.zip` from
     <https://github.com/jackwener/opencli/releases/latest>, unzip it (the folder has
     `manifest.json` at its top level), then `chrome://extensions/` → **Developer mode** on
     → **Load unpacked** → select that folder.
   - `opencli doctor` prints these steps itself when the extension is missing, so read its
     output back to the user rather than guessing.
   - **Bridge-wake quirk (non-obvious):** the MV3 service worker goes dormant after navigation/idle,
     so commands then fail with `Extension: not connected` / `BROWSER_CONNECT` even though it's
     installed. Fix: click **↻ reload** on the OpenCLI card in `chrome://extensions/`.
     `opencli daemon restart` alone does NOT wake a dormant worker. (Daemon runs on port 19825.)
   - **Policy fallback:** where the extension cannot be installed at all, OpenCLI can also
     bridge to a Chrome started with `--remote-debugging-port=9222`. Treat that as a last
     resort and tell the user what it changes.
4. **Bound tab is blank / wrong** (`state` shows `about:blank` or no URL) → ask the user to
   focus the real Chrome tab they want, then `opencli browser <slug> bind` again.

## Verify it's ready

```bash
opencli doctor
# [OK] Daemon: running on port NNNNN
# [OK] Extension: connected (vX.Y.Z)
```

Once green, `preflight.py` will bind the focused tab and assert a real URL. Only then drive.

## Notes

- Versions move; `doctor` will suggest `npm install -g @jackwener/opencli` and an extension
  update when newer ones exist. Updating is optional - the bind/state/eval/fill/upload surface
  this skill uses is stable. Don't block a run on an available-but-not-required update.
- Profiles: `opencli --profile <name> …` selects a Chrome profile alias if you run multiple.
