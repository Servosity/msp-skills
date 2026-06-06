# cipp skill - the MSP pain it closes

## The pain

CIPP (the CyberDrain Improved Partner Portal) is how a lot of MSPs run Microsoft
365 across every client tenant from one place. But CIPP scopes nearly every
endpoint by a single `tenantFilter` - the portal and the API work one tenant at a
time. The questions an MSP owner asks at QBR time are the opposite shape: *how
many users still lack MFA across all clients, which tenants drifted off our
security baseline, where are we paying for licenses nobody uses, who hasn't
signed in for 90 days anywhere.* None of those has a single screen.

Per the official [CIPP API documentation](https://docs.cipp.app/user-documentation/cipp/integrations/cipp-api),
the API authenticates with an Azure AD app registration over OAuth2 client
credentials and runs through Microsoft Graph, which throttles at scale. So the
two ways MSPs answer fleet-wide questions today both hurt: click through each
tenant in the portal by hand, or hand-roll PowerShell against the API and fight
HTTP 429 / Retry-After throttling - where a halted bulk change across dozens of
tenants means starting over. As of June 2026 there's no published single-binary
CLI or MCP server for CIPP that we could find, so an AI agent has nothing to
drive in one natural-language step.

## What this skill does about it

- **`cipp-cli fanout --endpoint /ListUsers --all-tenants --save`** - runs one read
  across every tenant at once, with throttle-aware backoff and resume-after-halt,
  into a local SQLite store. One command replaces the per-tenant click-through.
- **`cipp-cli posture --dimension mfa`** (also `ca`, `standards`, `bpa`) - the
  cross-tenant posture matrix the UI never renders, read instantly from the store.
- **`cipp-cli licenses waste`** - surfaces assigned-but-unused seats across all
  tenants so the next CSP bill stops paying for empty licenses.
- **`cipp-cli users stale --days 90`** - flags licensed accounts with no recent
  sign-in, fleet-wide, in one pass.
- **`cipp-cli bulk --from offboards.csv --execute`** - drives add-user / offboard /
  remove-user / set-forwarding from a CSV with 429 backoff and resume-after-429
  checkpointing, so a throttled batch continues instead of restarting.

## Status

Beta. Validated against the CIPP API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
