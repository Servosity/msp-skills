# NEEDS DAMIEN - one 10-minute auth to generate the first real report

Everything in `governance-snapshot` is built, tested, and armed **except** the one
thing an agent must not fake: a real token against a real tenant. This card is the
whole ask. Do it once and the first real Servosity governance report generates itself.

> **Status right now:** No live report exists. The demo report you can see today
> (`samples/demo-report.md`) is **synthetic sample data** - clearly-fake vendors
> (Acme, Contoso, Fabrikam). Nothing has touched Servosity's tenant yet.

---

## Step 1 - be signed in (30 seconds)

In your normal Chrome, be signed in to Microsoft 365 / Entra as a **Servosity tenant
admin** (Global Reader is enough - this is read-only, and Global Reader can consent to
read scopes). Leave that Chrome window open; the connect-tool drives *your* real
session, not a fresh browser.

## Step 2 - run connect-tool to mint a READ-ONLY Graph token (~8 minutes)

Run this in Claude Code:

```
/connect-tool mint a read-only Microsoft Graph token for the Servosity tenant and
store it in the macOS Keychain as service "MICROSOFT_GRAPH_TOKEN" account "servosity".
Use Graph Explorer (developer.microsoft.com/graph/graph-explorer) signed in with my
open Chrome session. Consent ONLY these read-only scopes, then copy the access token:
  - Application.Read.All            (read enterprise apps / service principals)
  - Directory.Read.All             (read the org + directory objects)
  - DelegatedPermissionGrant.Read.All  (read OAuth2 consent grants)
Prove it worked with:  GET https://graph.microsoft.com/v1.0/servicePrincipals?$top=1
```

**About the scopes - all three are READ-only.** Consenting them shows an admin-consent
prompt (they are `.Read.All` application/directory scopes); that prompt grants *read*
access only. None of them can change, create, or delete anything in the tenant. The
audit itself only ever issues `GET` requests.

> Graph Explorer tokens are delegated and last ~1 hour - perfect for a one-shot first
> run. For a recurring/unattended report, register an app-only (client-credentials)
> token with the same three read scopes later; the script reads whatever token is in
> the Keychain, so nothing else changes.

## Step 3 - run the single command (30 seconds)

```
playground/governance-snapshot/scripts/first-real-run.sh
```

That's it. The script will:
1. pull the token from the Keychain (never printing it),
2. verify auth with `microsoft-graph-cli doctor`,
3. run `microsoft-graph-cli apps consent --json` against Servosity's tenant,
4. **prove the output is not the canned demo** (it refuses if it matches the sample),
5. render `out/servosity-report-<time>.md`, and
6. write `out/receipts-<time>.md` - headline counts + the raw JSON, so every number is
   spot-checkable in **Entra admin center → Enterprise applications → Consent and
   permissions**.

## Verify it's real, not canned

The report and receipts land in `playground/governance-snapshot/out/`. The script prints
a `PROOF:` line showing the live output's hash differs from the demo sample's, and the
receipts embed the raw `apps consent --json` so you can reconcile the top-line counts
against the Entra portal yourself.

---

_Until Step 2 happens, `first-real-run.sh` stops cleanly at the auth boundary and the
live report does not exist. That is by design - no agent will claim otherwise._
