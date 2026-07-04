# Third-Party App Consent Report - Sample MSP Client

**DEMO - synthetic sample, not a real tenant.**  _Generated 2026-07-03 · source: `microsoft-graph-cli apps consent` (read-only)_

## Executive summary

**5 of 7 third-party apps need review, including 1 with privilege-escalation permissions.**

| Metric | Count |
| --- | ---: |
| Third-party apps consented | 7 |
| &nbsp;&nbsp;of which external (other tenants) | 6 |
| &nbsp;&nbsp;of which internal (homegrown) | 1 |
| Admin-consented (tenant-wide) | 5 |
| User-consented (shadow IT) | 2 |
| Hold application (app-only) permissions | 3 |
| Hold privilege-escalation permissions | 1 |
| Disabled but still consented | 1 |
| **High-risk (score ≥ 3)** | **5** |
| Microsoft first-party apps (excluded from findings) | 3 |
| Total service principals scanned | 10 |

## Findings (highest risk first)

| # | App | Origin | Risk | Flags | Application permissions | High-privilege delegated scopes |
| ---: | --- | --- | ---: | --- | --- | --- |
| 1 | Third-party app 1 | external | 7 | application-permissions, privilege-escalation, admin-consented | Application.ReadWrite.All (Microsoft Graph) ⚠escalation; User.ReadWrite.All (Microsoft Graph) | - |
| 2 | Third-party app 2 | external | 6 | application-permissions, high-privilege-delegated, admin-consented | Files.Read.All (Microsoft Graph); Mail.Read (Microsoft Graph) | **Files.Read.All, Mail.Read** (+1 more) |
| 3 | Third-party app 3 | external | 4 | application-permissions, admin-consented | Mail.ReadWrite (Microsoft Graph) | - |
| 4 | Third-party app 4 | internal | 4 | high-privilege-delegated, user-consented, disabled-but-consented | - | **Mail.ReadWrite** (+1 more) |
| 5 | Third-party app 5 | external | 3 | high-privilege-delegated, admin-consented | - | **Directory.Read.All, User.Read.All** |
| 6 | Third-party app 6 | external | 1 | user-consented | - | Calendars.Read, User.Read, offline_access |
| 7 | Third-party app 7 | external | 1 | admin-consented | - | User.Read, email, offline_access, openid ... |

## Recommended actions

- **privilege-escalation** - Revoke now or confirm it is a sanctioned admin tool. This permission is a standing path to full tenant control.
- **application-permissions** - Confirm each app-only permission is still needed; app-only access does not expire with a user and is easy to forget.
- **high-privilege-delegated** - Right-size the delegated scopes to least privilege; drop any read-all/write-all the vendor does not actually use.
- **user-consented** - Turn off end-user consent (or restrict it to verified publishers) so new shadow-IT apps require admin review.
- **disabled-but-consented** - Remove the consent grants for disabled apps - dead service principals should not retain access.

## What the flags mean

- **privilege-escalation**: Holds an application permission that can grant itself or others more access (Application.ReadWrite.All, RoleManagement.ReadWrite.Directory, AppRoleAssignment.ReadWrite.All, or Directory.ReadWrite.All). Treat as tenant-admin-equivalent.
- **application-permissions**: Runs as itself with NO signed-in user (app-only). App-only access is tenant-wide and unattended - the highest-trust consent class.
- **high-privilege-delegated**: Delegated consent includes broad read-all, any write-all, mail, or directory/role scopes.
- **admin-consented**: Tenant-wide consent (AllPrincipals) - applies to every user, not just whoever clicked.
- **user-consented**: Individual users consented this app themselves (shadow IT). Review your user-consent policy.
- **disabled-but-consented**: The service principal is disabled yet still carries consent - stale access that should be cleaned up.

---
_Read-only report. Every number above is spot-checkable in Entra admin center → Identity → Applications → Enterprise applications → Consent and permissions. Regenerate any time with `microsoft-graph-cli apps consent --json`._
