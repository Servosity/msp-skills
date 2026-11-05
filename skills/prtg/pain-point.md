# prtg skill - the MSP pain it closes

## The pain

Many MSPs give each client its own PRTG core. Paessler's own MSP offering
(https://www.paessler.com/msp-offering) says PRTG MSP Edition supports running
clients "on fully separate instances". Each core then has its own web interface,
its own alarm list and its own API key. The start-of-shift question "what is down
for every client right now?" means logging in to each core in turn, and the shift
handoff question "what changed overnight?" means scrolling each core's sensor lists.

The multi-server desktop client Paessler used to ship, the Enterprise Console, has
been unsupported since PRTG 19.4.53; Paessler points users to PRTG Desktop and PRTG
MultiBoard instead (Paessler blog, "PRTG Enterprise Console EOL - Switch to PRTG
Desktop Now"). Those are viewers for a person at a desk. An AI agent or a script
still has to call each core's API one by one, merge the answers, and notice when a
core did not answer at all.

Muting is the quiet failure. An acknowledgement or pause with no end date stays
silent until someone remembers it, and nothing lists every muted sensor across
cores, so a real outage can sit behind last month's "known issue".

## What this skill does about it

- `prtg-cli fleet alarms --status down --unacked --agent` - every unacknowledged
  down sensor on every core in one worst-first list, with core, device, priority
  and minutes down.
- `prtg-cli fleet handoff --since 12h --agent` - new alarms and recoveries since
  the last shift, including sensors that came back after a pause or
  acknowledgement.
- `prtg-cli fleet silenced --indefinite --older-than 7d --agent` - everything
  muted with no end date for more than a week, with who muted it and why.
- `prtg-cli fleet drift --within 30d --agent` - cores behind your newest PRTG
  version, licenses and maintenance ending soon, disconnected probes.
- `prtg-cli fleet pause --kind device --filter 'name = "fw01"' --duration 2h --message 'Patch night'` -
  one maintenance pause across every core, previewed per core until `--yes`.

## Status

Beta. Validated against the PRTG API surface and mock PRTG cores; the closed-loop
receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
