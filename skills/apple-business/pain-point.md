# apple-business skill - the MSP pain it closes

## The pain

Zero-touch deployment of Apple devices depends on one setting most MSPs never look at
until it fails: which MDM server each serial is assigned to in Apple Business Manager.
A device that was never assigned, or that sits in a product family with no default
server, simply does not enroll. A Jamf Community thread from July 2024 shows the shape:
a new MacBook Pro was assigned in Apple Business Manager, yet hours later "the serial
number does not appear in a PreStage Enrollment section"
([Jamf Community](https://community.jamf.com/t5/jamf-pro/can-t-assign-a-laptop-from-apple-business-manager-to-mdm/m-p/321792)).
The technician finds out at the client's desk.

The same shape repeats for AppleCare and Activation Lock. Both are per-device answers,
so a fleet view before a renewal conversation or a hardware refresh means looking up
serials one at a time. And every client is its own Apple Business organization with its
own API account, so nothing in the portal rolls them up.

## What this skill does about it

- `apple-business-cli devices unassigned --all-orgs` - every org-owned device not assigned
  to any MDM server across every client, oldest first, with `no_default_server` flagging
  the product families that have no default server.
- `apple-business-cli applecare expiring --within 90d --group-by order --all-orgs` - the
  devices whose AppleCare ends inside the window, grouped by purchase order for the
  renewal quote.
- `apple-business-cli devices locked --type user --all-orgs` - devices still Activation
  Locked to a user's Apple Account, with the owner when Apple's built-in device
  management data is synced.
- `apple-business-cli fleet summary` - one row per client org: devices, unassigned,
  AppleCare expiring, uncovered, user-locked and stale MDM servers.
- `apple-business-cli devices assign --server "Jamf Prod" --from-unassigned --dry-run` -
  preview moving the unassigned devices to the right MDM server, then send it once a
  person approves.

The fleet reports read a local store per API account, built by `apple-business-cli sync`
and `apple-business-cli devices enrich`; run both first (an unsynced store answers empty).

## Status

Beta. Validated offline against the Apple Business API surface (a fake server built from
Apple's public API reference); no live organization has run it yet. The closed-loop
receipt (a named MSP running it live in their production tenant at a Build Session) is
tracked separately and added here as `video.md` once it exists.
