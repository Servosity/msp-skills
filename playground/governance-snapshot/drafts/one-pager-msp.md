<!-- DRAFT — internal MSP one-pager for the Third-Party App Consent Review play. Not client-facing. Not final copy. -->

# The Third-Party App Consent Review — an MSP play (DRAFT)

**The offer in one line:** a read-only, same-day report that tells a client exactly which
outside apps can touch their Microsoft 365 mail, files, and directory — ranked by risk —
and what to revoke. Runs from one command. No agent installed in the tenant, no writes.

## Why this sells

- **Consent is the blind spot.** OAuth app consent is how modern M365 breaches persist —
  an attacker (or a careless vendor) gets a token that survives password resets and MFA.
  Most SMBs have never once reviewed what they have consented to.
- **It's invisible in the portal.** Entra spreads this across every enterprise app's
  permissions blade. Nobody clicks all of them. This report is the one screen that
  doesn't exist.
- **Read-only = zero-friction yes.** "We'll run a read-only report, change nothing, and
  show you the risks" is an easy approval — no project, no downtime, no risk.

## The play

1. Get a read-only Graph token for the client tenant (`Application.Read.All`,
   `Directory.Read.All`, `DelegatedPermissionGrant.Read.All` — all read).
2. `microsoft-graph-cli apps consent --json > audit.json`
3. `report.py audit.json --org "<Client>" -o report.md` → brand it with the kit → deliver.
4. Lead with the **privilege-escalation** and **admin-consented + high-privilege** rows.
5. Close on the managed **app-consent policy** as recurring MRR: you become the approval
   gate for new app consents, and you re-run the report [monthly/quarterly].

## Packaging ideas (to decide)

- **Free tenant health check** → funnel to a paid security posture engagement.
- **Add-on to the quarterly business review** — one page, high signal, renews the security
  conversation every quarter.
- **Included in a managed-security tier** as the recurring consent-governance control.

## What it is not (set expectations)

- Not a remediation tool — it reports; a human revokes in Entra. (By design: read-only.)
- Not a replacement for Conditional Access or a full CASB — it is the consent-surface
  slice, which is the cheapest high-value place to start.
- App-role (application permission) names resolve for the well-known high-risk Graph roles;
  an unusual app-role shows as an id and still flags — never silently dropped.

## Open questions for the offer (Damien)

- Price point / whether it's a free lead magnet vs. a paid line item.
- Cadence of the recurring review (monthly vs quarterly) and whether it's agent-automated.
- Whether to co-brand with Servosity or fully white-label for the MSP.

_Draft. Numbers and framing to be validated against a real Servosity-tenant run (see
`scripts/first-real-run.sh`) before this goes in front of anyone._
