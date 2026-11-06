# domain-cert-tracker skill - the MSP pain it closes

## The pain

Every client an MSP looks after owns domains and TLS certificates, and they
are spread across many registrars, DNS providers and certificate authorities.
The expiry list usually lives in a spreadsheet or a shared calendar that
someone keeps current by hand. MSPs keep asking for something better: an
r/msp thread titled ["Free SSL expiration tracker that makes
sense"](https://www.reddit.com/r/msp/comments/1ohanfn/free_ssl_expiration_tracker_that_makes_sense/)
is one example of the request.

The job is about to get harder. CA/Browser Forum ballot
[SC-081v3](https://cabforum.org/2025/04/11/ballot-sc081v3-introduce-schedule-of-reducing-validity-and-data-reuse-periods/)
cuts the maximum TLS certificate lifetime to 200 days from March 2026, 100
days from March 2027 and 47 days from March 2029. Renewal stops being a yearly
calendar entry and becomes a monthly automated job, and automation fails
quietly:

- **A renewed certificate that never reached the server.** The CA issued it
  and logged it in Certificate Transparency, but the web server still serves
  the old one until it expires and the client calls.
- **A certificate nobody ordered.** A new issuer for a client domain, or one
  the domain's CAA record does not permit, is visible in the public CT logs
  but only if someone searches them domain by domain.
- **A registration that lost its lock or is about to lapse.** Registrar
  portals show one account at a time.

Single-lookup tools (an RDAP client, a crt.sh search, an SSL checker) answer
one domain or one host at a time and keep no per-client grouping or history,
so nobody gets told when something changes.

## What this skill does about it

- `domain-cert-tracker-cli clients set "Acme Dental" --domain example.com --host www.example.com --yes`:
  one local inventory of every client's domains and hosts.
- `domain-cert-tracker-cli sweep --agent`: checks registrations (RDAP),
  Certificate Transparency, the live certificate on each host and DNS for the
  whole inventory, and records what changed as typed events.
- `domain-cert-tracker-cli report --days 30 --agent`: per client, what expires
  within 30 days, which registrations lost their lock, new CT certificates and
  DNS drift, plus every check that could not be verified.
- `domain-cert-tracker-cli renewals --agent`: hosts still serving an old
  certificate after a newer one was logged, or past their renewal point with
  no successor.
- `domain-cert-tracker-cli issuers audit --days 7 --agent`: certificates from
  an issuer the domain's CAA does not permit, or a first-ever issuer, separated
  from routine renewals.

No account or credential is needed anywhere: every source is public.

## Status

Beta. Tested against the public RDAP, crt.sh, DNS and TLS services it reads;
the closed-loop receipt (a named MSP running it on their own client list at a
Build Session) is tracked separately and added here as `video.md` once it
exists.
