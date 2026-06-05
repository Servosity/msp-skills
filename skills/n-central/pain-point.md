# n-central skill - the MSP pain it closes

## The pain

Three pains, two documented by N-able itself and one by the community project
that exists precisely because integrators kept hitting it:

**1. Cross-customer questions mean console walks.** Active issues, device
locations, and property values live under each org unit in the N-central
console. "What's red right now across everyone," "where is EXCHANGE01," and
"which devices are missing the Backup Plan property" are per-customer clicks -
and at MSPs running more than one N-central server, the walk repeats in every
console. Scripting around it hits N-able's documented per-endpoint concurrency
caps: the official REST API known-issues page states limits vary from 3 to 50
concurrent calls and the server answers 429 beyond them.

**2. The JWT dies silently every 90 days.** N-central API access is a JSON Web
Token generated for an API-only user - and that user's password expiry
(default 90 days) invalidates the JWT without any signal to the integration
using it. The community-maintained NC-API-Documentation project on GitHub
(AngryProgrammerInside) documents the trap plainly: an expired password for
the originating account blocks JWT access, regenerating a token invalidates
the previous one, and role changes require a regen. Most teams discover all of
this when a dashboard goes quiet.

**3. Errors that look like success.** N-able's official known-issues page
admits that "in a few cases, the error response from N-central's core engine
appears as an Error Message attached to a 200 OK response." A script that
checks HTTP status codes - which is to say, nearly every script - processes
the error body as data.

## What this skill does about it

- `n-central-cli triage --by customer` - the morning NOC sweep as one command:
  active issues across every org unit, grouped and ranked by severity.
- `n-central-cli whereis EXCHANGE01` - full path (server > service org >
  customer > site) from the local mirror, offline, in milliseconds.
- `n-central-cli fanout "acme"` - one full-text query across every synced
  server mirror; each match tagged with the server it came from. The
  multi-tenant search no console has.
- `n-central-cli guardian --password-set 2026-03-01` - validates the token,
  warns 14 days before password expiry kills the JWT, and scans for the
  200-OK-with-error-body case. Exits non-zero so CI can gate on it.
- `n-central-cli props audit --required "Backup Plan"` - custom-property
  coverage as a percentage by customer, with the devices still missing it.
- `n-central-cli maint coverage --before 2026-06-15` - devices and sites with
  no maintenance window before the patch wave, so nothing reboots in business
  hours.

whereis, fanout, and search run against a local SQLite mirror (`sync` first),
so they are instant, offline, and immune to concurrency caps.

## Sources

- N-able, "REST API known issues and limitations" (official documentation;
  developer.n-able.com/n-central/docs/rest-api-known-issues-and-limitations and
  documentation.n-able.com) - per-endpoint concurrency caps (3-50, 429 beyond),
  error messages attached to 200 OK responses, single-device task creation.
- AngryProgrammerInside/NC-API-Documentation (community GitHub project) -
  password expiry (90 days default) blocking JWT access, token regeneration
  invalidating prior tokens, role changes requiring regen.
- N-able N-central documentation, "How to access and use N-central REST APIs" -
  the API-only-user + Generate JSON Web Token flow this skill's auth setup
  mirrors.

## Status

Beta. Validated against the N-able N-central REST API surface; the closed-loop
receipt (a named MSP running it live in their production tenant at a Build
Session) is tracked separately and added here as `video.md` once it exists.
