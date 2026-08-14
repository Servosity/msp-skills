# unifi-network skill - the MSP pain it closes

## The pain

UniFi gear is everywhere in small-business networks, and the thing operators keep
asking it for is the one thing it will not give them: history. The Network
integration API exposes no config-versioning and no audit-trail endpoint, so
"what changed on this site, and when?" has nothing to query. The Ubiquiti
Community has carried standing feature requests on exactly this for years - threads
titled [UniFi Change Logs or Change Control options?](https://community.ui.com/questions/UniFi-Change-Logs-or-Change-Control-options/7c9f7b06-9c3b-4cad-92a7-5920b06e9f9c),
[UniFi audit/change logs supported?](https://community.ui.com/questions/UniFi-audit-change-logs-supported/64ced74e-114d-4c2e-9e8d-469388b9eccc),
[Audit log of recent changes](https://community.ui.com/questions/Audit-log-of-recent-changes/710d01da-2191-4acf-84f0-ec4ca830eed7),
and [Controller Logs for settings changes](https://community.ui.com/questions/Controller-Logs-for-settings-changes/c3dbb64b-78ca-48a7-92de-b78ec8008e1d).

The same missing-baseline problem shows up twice more. There is no first-seen
record for a device or client, so nothing on the screen distinguishes hardware
that appeared this morning from hardware that has been there a year - you are
eyeballing a list you have no reference point for. And per-port interface data
never appears in any list response, only in a per-device detail fetch, so
answering "which ports are free, and which already energize PoE?" means opening
every switch on the site one at a time.

None of this is hard work. It is just work the console makes you redo by hand
every time, because the state you would compare against was never kept.

## What this skill does about it

- **`unifi-network-cli drift --site default --json`** - diffs the site's networks,
  firewall, WiFi, and DNS config against a snapshot this command captured the last
  time it ran, then advances the snapshot. It keeps its own history precisely
  because the API has none to read.
- **`unifi-network-cli newcomer --since 7d --json`** - holds a first-seen record per
  device and client, so new hardware surfaces against a real baseline instead of a
  flat list. The first run for a site becomes the baseline rather than dumping the
  whole network as new.
- **`unifi-network-cli port-audit --site default --json`** - per-port link state and
  PoE status for every switching or gateway device on the site, reading the device
  list from the local mirror and fetching the interface detail the list endpoints
  omit. Without `--json` the terminal path prints a one-line summary per device.
- **`unifi-network-cli rule-predict --src 10.0.3.50 --dst 10.0.0.1`** - walks the
  synced firewall policies in the gateway's own ascending-index, first-match-wins
  order and reports which one would match, flagging zone-wide and unresolvable
  policies as uncertain rather than guessing. Pass host IPs - a CIDR is tested as its
  first address only.
- **`unifi-network-cli topology --site default`** - groups every synced client under
  the device it is attached to, so "which clients are behind this AP?" is one
  command against local data. Device-to-device uplink chaining is not exposed by the
  list endpoints, so every device is listed at the top level rather than nested.

## Scope

Single self-hosted UniFi OS gateway, via the local Network integration API at
`https://<gateway>/proxy/network`. Not the Site Manager cloud API, and not a
multi-tenant view across deployments. UniFi Protect and UniFi Access are separate
APIs and are out of scope.

## Status

Awaiting live verification. The command surface is validated against the UniFi
Network integration API's published spec and the CLI's own mock verification suite; the closed-loop
receipt - a named MSP running it against a production gateway - is tracked
separately and lands here once it exists.

## Attribution

The underlying CLI was generated and contributed by Ricardo Cabral
([@phoenix-server](https://github.com/phoenix-server)) and is redistributed here
under Apache-2.0 with the original `NOTICE` preserved in
[`cli/NOTICE`](./cli/NOTICE).
