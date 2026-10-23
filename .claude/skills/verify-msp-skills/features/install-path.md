# Install path

A user installs a connector by running `skills/<slug>/install.sh`, which
resolves the connector's newest sealed GitHub release, downloads `<slug>-cli`
and `<slug>-mcp` for the platform, verifies each against its `.sha256`
sidecar and puts both in `~/.local/bin`. The user then runs `<slug>-cli
--version` and `<slug>-cli doctor`.

## Sub-features

- `install-sealed` installs both binaries from a release reported as sealed (immutable).
- `install-verified` installs only binaries whose sha256 matches the sidecar.
- `cli-version` prints the CLI name and the manifest's version.
- `mcp-help` prints the MCP server's flags (`-transport`, `-addr`) and exits 0.
- `cli-doctor` prints a JSON health report and exits 0.

## How to get to it (user POV)

- `curl -fsSL <raw install.sh URL> | bash` (macOS, Linux), as each connector's README shows.
- `bash skills/<slug>/install.sh` from a checkout.
- `<slug>-cli --version`, `<slug>-mcp --help`, `<slug>-cli doctor --json` afterwards.

## Driving it with verify.py

Preconditions:

- `verify.py doctor --slug <slug>` exits 0.
- The run has built `<slug>-cli` and `<slug>-mcp` from this checkout with release.yml's ldflags.

- **Serve a sealed release.** The run copies the built binaries to
  `<slug>-cli-<os>-<arch>` and `<slug>-mcp-<os>-<arch>`, writes their
  `.sha256` sidecars, and starts `tools/maintainer/installer_fixtures/fake_github.py`
  with tag `<slug>-v<manifest version>` (it reports `"immutable": true`).
  `f1-install/fake-github.log` lists every request the installer made.
- **Install.** Run `verify.py run --slug <slug> --features 1`. It runs
  `bash skills/<slug>/install.sh` with `MSP_SKILLS_API_BASE` set to the
  fixture and HOME set to the scratch HOME. `f1-install/install-sh.json`
  shows exit code 0 and `Release: <slug>-v<version> (sealed)`; both
  binaries exist in the scratch `~/.local/bin` with the built sha256.
- **Run them.** The same run executes `<slug>-cli --version`,
  `<slug>-mcp --help` and `<slug>-cli doctor --json`
  (`cli-version.json`, `mcp-help.json`, `cli-doctor.json`). Each exits 0;
  doctor prints a JSON object naming the fake vendor's base URL.
- **Proof.** `SUMMARY.txt` line `1 install PASS` with the sealed tag, the
  version string and the doctor key count.

## Gotchas

- The installer never touches the real `~/.local/bin`: INSTALL_DIR defaults to
  `$HOME/.local/bin`, and HOME is the scratch HOME. The isolation fingerprint
  covers the real `~/.local/bin/<slug>-cli` and `<slug>-mcp` too.
- This proves the installer against binaries built from this checkout, not the
  published release. `bash tools/maintainer/verify_all.sh --slug <slug>`
  already dry-runs `install.sh` against the real release URLs.
- If install.sh fails, features 2 to 6 still run on the built binaries and the
  summary says so; feature 1 stays FAIL.
- A version string that differs from the manifest is noted, not failed: the
  release injects it through ldflags and a reprint can move the variable.
