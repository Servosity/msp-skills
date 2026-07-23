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
3. **Extension not connected** (`opencli doctor` has no `[OK] Extension`) → the Chrome
   extension must be installed and Chrome open:
   - Download the latest extension from <https://github.com/jackwener/opencli/releases>.
   - Load it in Chrome (Extensions → enable Developer mode → Load unpacked, or the packaged
     `.crx` per the release notes), then keep Chrome open.
   - Re-run `opencli doctor` and **loop until `[OK] Extension: connected`** before any nav.
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
