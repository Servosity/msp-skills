# connectwise-control skill - the MSP pain it closes

## The pain

ConnectWise Control (formerly ScreenConnect) is the remote-support tool a huge number
of MSPs live in - and it is a web console built for a technician driving one remote
session at a time. On r/msp and in the ConnectWise/ScreenConnect community, the
recurring ask is the same: there is no first-party CLI and no terminal-native way to
work with sessions in bulk. Answering "which machines have an open session right now?",
"what happened on this endpoint?", or "run this one diagnostic across these five
machines" means scrolling the console, clicking into each session, and typing the same
command into each command box by hand.

For an MSP, remote control is the delivery mechanism for support. The console is great
for the live, eyes-on session - but the moment you want to script, search, or hand the
work to an AI agent, the console is a wall.

## What this skill does about it

It turns the ConnectWise Control instance surface into typed commands an agent can drive,
with an offline SQLite mirror for fast session lookups:

- **`sessions list --session-type Access`** - every access session across a group, as JSON, without scrolling the console.
- **`sessions get-detail --session-id <id>`** - one session's connections and recent events in a single call.
- **`audit query-log --session-name <name>`** - what happened on a machine, from the audit log, by session and time window.
- **`sessions run-command --session-id <id> --command <cmd>`** - run a diagnostic or remediation command on a guest endpoint (gated human-in-the-loop - it executes a real command on the machine).
- **`security get-configuration`** - the instance's users and roles in one read.

## Status

Beta. Validated against the ConnectWise Control instance API surface; the closed-loop
receipt (a named MSP running it live against their own instance at a Build Session) is
tracked separately and added here as `video.md` once it exists.
