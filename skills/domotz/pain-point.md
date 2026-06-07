# domotz skill - the MSP pain it closes

## The pain

Network-monitoring discussions on r/msp keep surfacing the same gap with per-site
monitoring tools: visibility is organized one site (Collector) at a time, but the
questions an MSP owner actually asks are fleet-wide. "Which of my clients has
something down right now?" "What appeared on a network overnight?" "Give me one
asset list for the QBR." In the Domotz portal each of those means clicking through
every Collector by hand or exporting per-site and merging spreadsheets. The data is
all there - it just doesn't roll up across clients in a single view.

## What this skill does about it

The skill syncs every Collector into a local mirror and turns the cross-site
questions into one command each:

- **`fleet health`** - one status board across every site: agent status and
  down-device counts, so "is anything on fire?" is a single glance.
- **`fleet offline`** - every offline or unreachable device across all sites,
  prioritized, instead of paginating each agent.
- **`fleet new --since 24h`** - devices first-seen anywhere in the fleet in a time
  window: a rogue-device security sweep across all clients at once.
- **`fleet inventory --csv`** - one asset table (vendor, model, type, OS, serial,
  site) for the whole fleet - the QBR/CMDB export in one line.
- **`fleet unmonitored`** - devices Domotz can't fully monitor (auth/SNMP coverage
  gaps), surfaced fleet-wide so monitoring blind spots don't go silent.

## Status

Beta. Validated against the Domotz API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
