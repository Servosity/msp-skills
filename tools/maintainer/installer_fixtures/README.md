# installer fixtures

Behavioural proof for `skills/<slug>/install.sh` and `install.ps1` (one script
rendered per slug; `render_fleet.py --check` proves all 134 files identical
modulo slug). Run locally:

    bash tools/maintainer/installer_fixtures/run.sh          # macOS / Linux
    powershell -File tools/maintainer/installer_fixtures/run.ps1   # Windows (5.1); pwsh elsewhere is a smoke test

CI: `.github/workflows/installer-fixtures.yml` (ubuntu-latest bash job,
windows-latest Windows PowerShell 5.1 job).

Files: `fake_github.py` (stdlib stand-in for api.github.com + github.com on one
port, pointed at through `MSP_SKILLS_API_BASE`), `run.sh` / `run.ps1` (the
cases), `harness.ps1` (fresh-process runner with Move-Item failure injection),
`render_fleet.py` (`--check` / `--from-template DIR`).

## Sealed-release check: documented boundary

The installers require the release object from `/releases/tags/<tag>` to carry
a top-level `"immutable": true`. In bash that field is read by a small awk JSON
validator (`top_level_immutable`); in PowerShell by `ConvertFrom-Json`. The
contract is deliberately one-sided: any document the validator cannot accept as
one well-formed object makes the installer REFUSE (ambiguous), never accept.
Two boundaries are by design, not defects:

- an escape-encoded top-level key (for example `"\u0069mmutable"`) is rejected
  outright rather than decoded, so it can neither match nor hide a duplicate;
- bytes outside JSON's alphabet (NUL, control bytes other than tab/LF/CR) and
  nesting deeper than 64 levels are refused before parsing. The body arrives
  from api.github.com over TLS; a release object is three levels deep.

`run.sh` carries the unit probes of the extracted function (every shape the
adversarial reviews produced), so a regression in the validator fails CI.
