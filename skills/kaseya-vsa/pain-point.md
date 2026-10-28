# kaseya-vsa skill - the MSP pain it closes

## The pain

"What is missing, and where?" is the question an MSP asks every Patch Tuesday and
before every QBR. In Kaseya VSA 10 the answer is spread across policies and
organizations. Kaseya's own release note for VSA 10 patch status reports says
reporting "is currently limited to one policy at a time"
([Advanced reporting: Patch status reports](https://www.kaseya.com/platform/innovations/advanced-reporting-patch-status-reports/),
February 2026). A fleet-wide view therefore means one report per policy and a
spreadsheet to join the exports.

The same shape repeats for health: offline agents, disabled antivirus, a switched-off
firewall and full disks show up one organization or one device at a time, so the MSP
hears about a dead agent when the client calls. And running a script means starting it
in the console, waiting, and opening the execution log to read the output.

## What this skill does about it

- `kaseya-vsa-cli patch-exposure --by kb --severity critical,important` - every missing
  update ranked by how many devices lack it, with CVSS, CVEs and days since release, across
  every client in one answer.
- `kaseya-vsa-cli patch-exposure --by org` - which clients carry the most exposure, ready
  for the QBR conversation.
- `kaseya-vsa-cli health --check offline,av,firewall,disk` - every device failing a check,
  with per-check counts, so a dead agent is found before the client calls.
- `kaseya-vsa-cli software "Huntress Agent" --absent` - every device with inventory that is
  missing the agent you require.
- `kaseya-vsa-cli scripts run-wait <script-id> --device FRONTDESK-01` - run a script and get
  its exit code and output back in one call.

The fleet questions read a local mirror built by `kaseya-vsa-cli sync`, so they cost no
API calls once synced and stay inside VSA 10's hourly per-token rate limit.

## Status

Beta. Validated against the Kaseya VSA 10 API surface (a mock built from Kaseya's public
API reference at api.vsax.net); no live tenant has run it yet. The closed-loop receipt (a
named MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
