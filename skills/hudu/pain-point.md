# hudu skill - the MSP pain it closes

## The pain

"How do you keep your documentation from going stale?" is a perennial thread on
r/msp - and the honest answer is usually "we don't, until it bites us." Hudu is
where MSPs put their IT documentation, but the platform is a great place to *store*
hygiene problems, not to *find* them:

- Assets get created with half their required layout fields blank, and nobody
  notices until a tech is mid-incident and the detail they need was never filled in.
- Hudu has **no native password-expiration tracking**, so vault credentials quietly
  age past any sane rotation policy across dozens of client vaults.
- SSL certificates, domains, and warranties expire one company at a time, buried in
  the portal - you usually learn about a lapse when the client's site goes down.

Every one of these is a cross-tenant question, and the Hudu portal answers one
company on one page at a time. So the audit that should take a minute turns into an
afternoon of clicking, and most MSPs simply never do it.

## What this skill does about it

Sync your instance into a local mirror once (`hudu-cli sync`), then ask the
questions across every client at once:

- `hudu-cli audit completeness --agent` - rank companies by how completely their
  assets fill the required fields their layouts define, worst-first.
- `hudu-cli audit stale-passwords --older-than 180d --agent` - find vault entries
  nobody has rotated, grouped by company (rotation age is computed locally; the
  secret value is never read or stored).
- `hudu-cli audit expirations --within 30d --agent` - one typed, sorted list of
  SSL, domain, warranty, and password expirations coming due across all tenants.
- `hudu-cli audit stale-articles --older-than 365d --agent` - surface knowledge-base
  articles that haven't been touched in so long they probably no longer match reality.
- `hudu-cli audit summary --agent` - a single worst-first hygiene scorecard so you
  know which client to fix first.

## Status

Beta. Validated against the Hudu API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
