# OpenCLI bootstrap - install + configure when missing or disconnected

`scripts/preflight.sh` runs `opencli doctor` and refuses to proceed unless it shows
`[OK] Daemon` **and** `[OK] Extension`. When it doesn't, offer to set it up. Never fall
back to a spawned browser (Playwright, Puppeteer, headless Chromium): it carries no login.

## Decision tree

1. **`opencli` binary missing** (`command -v opencli` fails) → offer to install:
   ```bash
   npm install -g @jackwener/opencli     # node + npm are prerequisites (Homebrew: brew install node)
   ```
2. **Daemon not running** (`opencli doctor` has no `[OK] Daemon`) → the daemon auto-starts on
   first `opencli browser …` use; if it's wedged: `opencli daemon restart`.
3. **Extension not connected** (`opencli doctor` reports `[MISSING] Extension` or anything
   other than `[OK] Extension`) → the Chrome extension must be installed and Chrome open.
   It is NOT installed from the Chrome Web Store; it is loaded unpacked from the release zip:
   - Open <https://github.com/jackwener/opencli/releases/latest> and download the asset
     named `opencli-extension-v<version>.zip` (one zip; the version moves, the name pattern
     does not).
   - Unzip it. The unzipped folder contains `manifest.json` at its top level; that folder is
     what Chrome loads.
   - In Chrome: `chrome://extensions/` → toggle **Developer mode** on (top right) →
     **Load unpacked** → select the unzipped folder. Keep Chrome open.
   - Re-run `opencli doctor` and **loop until `[OK] Extension: connected`** before any nav.
   - `opencli doctor` prints these same steps itself when the extension is missing, so read
     its output back to the user rather than guessing.
   - **Bridge-wake quirk (non-obvious):** the MV3 service worker goes dormant after navigation/idle,
     so commands then fail with `Extension: not connected` / `BROWSER_CONNECT` even though it's
     installed. Fix: click **↻ reload** on the OpenCLI card in `chrome://extensions/`.
     `opencli daemon restart` alone does NOT wake a dormant worker. (Daemon runs on port 19825.)
4. **Bound tab is blank / wrong** (`state` shows `about:blank` or no URL) → ask the user to
   focus the real Chrome tab they want, then `opencli browser <slug> bind` again.

## Verify it's ready

```bash
opencli doctor
# [OK] Daemon: running on port NNNNN
# [OK] Extension: connected (vX.Y.Z)
```

Once green, `preflight.sh` will bind the focused tab and assert a real URL. Only then drive.

## Notes

- Versions move; `doctor` will suggest `npm install -g @jackwener/opencli` and an extension
  update when newer ones exist. Updating is optional - the bind/state/eval/fill/upload surface
  this skill uses is stable. Don't block a run on an available-but-not-required update.
- Profiles: `opencli --profile <name> …` selects a Chrome profile alias if you run multiple.
