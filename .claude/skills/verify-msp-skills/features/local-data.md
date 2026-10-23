# Local data

Connectors keep API responses on disk (a SQLite store and an HTTP response
cache). A user must be able to see where (`doctor`), erase it
(`local-data purge`, a dry run unless `--yes`), and run without it
(`<PREFIX>_NO_LOCAL_DATA=1` refuses `sync`, which exists to copy API data to disk).

## Sub-features

- `doctor-store-path` names the store location under the user's HOME.
- `purge-preview` lists what would be deleted and deletes nothing.
- `purge-apply` deletes the listed store with `--yes`.
- `no-local-data-sync` refuses `sync` before any request when the switch is set.

## How to get to it (user POV)

- `<slug>-cli doctor --json` and read `paths.data.dir` (4.34) or the store's
  `.db` path (older builds, for example `cache.db_path`).
- `<slug>-cli local-data purge`, then `<slug>-cli local-data purge --yes`.
- `<PREFIX>_NO_LOCAL_DATA=1 <slug>-cli sync`, or the same variable in the MCP server's env block.

## Driving it with verify.py

Preconditions:

- Earlier features may have left cache or store files in the scratch HOME;
  the sync check compares `.db` files before and after, so that is fine.

- **Store path.** Run `verify.py run --slug <slug> --features 6`. It runs
  `<slug>-cli doctor --json` (`f6-local-data/doctor.json`). A store path under
  the scratch HOME is PASS; a store path outside it is ERROR (isolation);
  none is FAIL.
- **Purge preview and apply.** If `local-data purge --help` exits 0, the
  harness writes a stand-in `data.db` (scaffolding, removed afterwards) in the
  store dir, runs `local-data purge --json` (must exit 0, list the file, keep
  it), then `local-data purge --yes --json` (must exit 0 and delete it).
  `purge-preview.json`, `purge-apply.json`. No `local-data` command is FAIL.
- **Sync refused.** If `sync --help` exits 0, the harness runs
  `<slug>-cli sync --json` with `<PREFIX>_NO_LOCAL_DATA=1`
  (`sync-no-local-data.json`). PASS needs a non-zero exit that names the
  variable, no vendor request and no new `.db` file. No `sync` command is SKIP.
- **Proof.** The verdict reason carries all three sub-checks.

## Gotchas

- The stand-in `data.db` is verification scaffolding, written and removed
  inside the scratch HOME only.
- `purge --yes` deliberately deletes nothing under a test harness; the
  driven environment sets no harness variable, so the user path is what runs.
- A connector whose `sync` is a no-op still writes an empty store; with the
  switch ignored that is the recorded FAIL.
- `--dry-run` is exempt from the sync refusal, so the harness never passes it here.
