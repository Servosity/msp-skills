# Changelog

All notable changes to this skill are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[semantic versioning](https://semver.org/).

## [0.1.9] - unreleased

### Security
- **Rebuilt with Go 1.26.11, which fixes a crash in Go's HTTP/1 server (GO-2026-6859).** A race in `net/http`'s `MaxBytesReader` could crash a Go HTTP server on a crafted request. No dependency changed.

## [0.1.8] - 2026-11-10

### Fixed
- **Writing to the local store costs almost no CPU.** The credential check on rows written to the local store read every row through each decoder it knows. It now refuses a row only when the row carries a secret a vendor response returned (raw or JSON-escaped) and stores every other row unread. The local store is no longer searched for this program's own credentials; what the CLI prints, writes to other files or sends is checked exactly as before (#612).

## [0.1.7] - 2026-11-09

### Changed
- **Lower startup cost.** The credential check over the program's own fixed text and the local store's fixed SQL statements reads its verdict from a table built for this connector, instead of decoding that text again in every process (about 0.5 CPU-s at startup and 0.4 CPU-s on each store open). A credential in that text is withheld exactly as before (#662).

### Fixed
- **Local store memory no longer climbs.** The credential check on rows written to the local store no longer grows with each page or spikes on one large record. The check is exactly as strict as before (#612).

## [0.1.6] - 2026-11-06

### Security
- **Rebuilt on Printing Press 4.35.1 ([GHSA-jh26-vhqh-692c](https://github.com/advisories/GHSA-jh26-vhqh-692c)).** The MCP server's HTTP mode listens on `127.0.0.1:7777` by default, refuses to start until `RIVERSIDE_FM_MCP_HTTP_TOKEN` is set (at least 32 characters), requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert` and `--tls-key` for any address other than loopback.
- **Adopted the shared credential kit.** Session cookies, the Spacefast token and the MCP HTTP token are withheld from every output, file and child process, and refused on any request to another host or over plain http.
- **Studio join secrets never leave the client.** `producerSecret`, `guestSecret` and `shareToken` are removed from every API response before any command sees it.
- The cookie jar is written atomically and privately, so a crash cannot leave it truncated.
- `RIVERSIDE_FM_MCP_READ_ONLY=1` hides `compute_per_speaker_talktime_stats` and every tool that writes the local learning store.

### Added
- `RIVERSIDE_FM_NO_LOCAL_DATA=1` keeps no local copy of Riverside data: reads go to the API, the response cache is off, and nothing new is written to the local store. Declared on every install channel with `RIVERSIDE_FM_MCP_READ_ONLY` and `RIVERSIDE_FM_MCP_HTTP_TOKEN`.
- `local-data purge` previews the local store files, response cache and learning journal it would erase, and deletes them only with `--yes`.

### Fixed
- **`media refresh` and `clips harvest` exit non-zero when a row fails.** Every row still prints with its status; a fetch, download or path failure now fails the run. `bulk export` counts a take as checkpointed only after its resume file is written.

- **`bulk export`, `ready`, `stale`, `media refresh` and `clips list` read every page.** They stopped at the first page, so a studio with more than 200 projects or a project with more than 100 takes was cut short with no warning.
- **Long titles no longer collide on disk.** A name over 80 bytes kept only its start, which dropped the id and extension, so two items with a long shared title landed on one path and one overwrote the other. The name now keeps its id and extension, and is always valid UTF-8.

- `bulk export` help and docs say what it writes: each take's transcript, asset list and HLS manifests, with its resume cursor in `<out>/.resume.json`. It downloads no audio or video; use `grab` for those.

- `doctor` no longer reports the API as reachable when its probe answers with an error status; it names `RIVERSIDE_FM_BASE_URL` instead.

### Removed
- `sync`, `export`, `import` and `analytics`. Every Riverside endpoint is nested under a studio, project or take id, so the press no longer renders them; the published `sync` had nothing to sync and the spec has no write endpoint for `import`. Use `bulk export`, `grab` and `search`.

## [0.1.5] - 2026-10-23

### Changed
- `cli/NOTICE` now names the product `riverside-fm-cli`, the binary this connector installs, instead of the generator's internal `-pp-cli` name. No code change.

## [0.1.4] - 2026-10-22

### Security
- The MCP server no longer forwards a flag whose value names a file or directory on the machine that runs it (for example --db, --input, --output, --config, --home), and refuses any argument the tool schema does not list; both are dropped from the MCP tool schemas and refused by name, so a tool call can neither read or write a local path nor reach another command's file flag. The command line keeps every flag.

## [0.1.3] - 2026-10-21

### Security
- **Rebuilt with Go 1.26.10, which distrusts compromised intermediate certificates in `crypto/x509` (GO-2026-6809).** Go now rejects those certificates itself when it verifies the vendor's HTTPS certificate, so the connector no longer depends on the system trust store having been updated to reject them. No other dependency changed.

## [0.1.2] - 2026-10-15

### Changed
- Rebuilt with Go 1.26.9 (was 1.26.6). Go 1.26.9 includes upstream Go security fixes,
  among them the `net/http` advisories GO-2026-6617, GO-2026-6613 and GO-2026-6612.
  No functional changes.

## [0.1.1] - 2026-09-18

### Changed
### Added
- **`RIVERSIDE_FM_NO_CONFIG_WRITE=1` keeps credential material off disk.** The rotated
  session cookie stays in memory for the running command and is never written to
  `config.toml`; `auth login` refuses to persist under the switch; wipes still run. Declared
  in the bundle prompt, the MCP Registry entry and the manual-install JSON.

### Fixed
- **The MCP server resolves its config the way the CLI does.** It used a hard-coded
  `~/.config/riverside-fm-cli/config.toml` and ignored `--config` and the switch above.
- **Windows bundle could not find its companion CLI.** The sibling lookup now tries
  `riverside-fm-cli.exe` first on Windows, then the bare name, then `RIVERSIDE_FM_CLI_PATH`,
  then `PATH`.

### Changed
- **The Claude Desktop bundle now carries the companion CLI.** Every MCP tool runs the
  CLI under the hood, but the `.mcpb` used to contain only the MCP server, so a one-click
  install on a machine without the CLI failed at the first tool call. The bundle now ships
  both binaries side by side for macOS (universal), Windows x64 and Linux x64. Older
  bundles are unchanged; this release's bundle is the first to include it.
- **The MCP server reports its real version.** `serverInfo.version` used to read
  `0.0.0-dev` (or a hard-coded literal) in every released MCP binary because the release
  build stamped the version into the CLI only. The release now stamps both and checks the
  built server's `initialize` reply against the tag before publishing.
- **Installers verify what they download.** `install.sh` and `install.ps1` now require a
  sealed (immutable) release, verify both binaries against the release's SHA-256 sidecars
  before touching anything, and replace the old binaries in a single transaction that is
  undone if any step fails. (Served from the repository, so this applies to every install
  from now on, not only this version.)

## [0.1.0]

### Added
- Initial msp-skills release: Riverside CLI + MCP server for exporting your own
  Riverside.com account.
- `bulk export` - archive a whole studio's transcripts, assets, and HLS manifests
  to disk with a resume cursor.
- `grab` - priority-fallback download of a recording (transcript, then audio,
  then HLS video).
- `transcripts convert` / `transcripts talktime` - convert transcripts to
  VTT/SRT/TXT/JSON/Markdown and compute per-speaker talktime.
- `search` - SQLite FTS5 full-text search across the local mirror of your synced
  transcripts, projects, and recordings.
- `clips harvest` and `media refresh` - pull Magic Clips and refresh short-lived
  CloudFront signed URLs before they expire.
- `ready` / `wait` - check or block on take readiness (backup, transcription, AI).
- Cookie-session auth (`auth login --chrome`) for Pro / Live / Webinar accounts
  that can't issue a Business-plan API key.
