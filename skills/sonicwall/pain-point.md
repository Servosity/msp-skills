# sonicwall skill - the MSP pain it closes

## The pain

SonicWall firewalls are common at the small businesses MSPs look after, and
their SSL-VPN is the part attackers went after. In August 2024 SonicWall
disclosed CVE-2024-40766 ([SNWLID-2024-0015](https://psirt.global.sonicwall.com/vuln-detail/SNWLID-2024-0015)),
an improper access control flaw in SonicOS management access and SSL-VPN; CISA
added it to its Known Exploited Vulnerabilities list on 9 September 2024. From
July 2025 Akira ransomware operators hit SonicWall SSL-VPN accounts again
([BleepingComputer, 1 Aug 2025](https://www.bleepingcomputer.com/news/security/surge-of-akira-ransomware-attacks-hits-sonicwall-firewall-devices/)).
SonicWall's own notice said it had "high confidence that the recent SSLVPN
activity is not connected to a zero-day vulnerability" and that "many of the
incidents relate to migrations from Gen 6 to Gen 7 firewalls, where local user
passwords were carried over". Its advice: update firmware to 7.3.0, reset all
local user passwords for accounts with SSL-VPN access, and enforce MFA
([SonicWall notice, 4 Aug 2025](https://www.sonicwall.com/support/notices/gen-7-and-newer-sonicwall-firewalls-sslvpn-recent-threat-activity/250804095336430)).
The MSP-facing advisory from SmarterMSP repeated it, down to "Reset passwords
for all local user accounts with SSLVPN access"
([SmarterMSP, 7 Aug 2025](https://smartermsp.com/cybersecurity-threat-advisory-sonicwall-ssl-vpn-targeted-by-akira-ransomware/)).

That advice assumes an MSP can answer three questions across every client the
same day: which firewalls run old firmware, which have SSL-VPN on a WAN zone,
and which local SSL-VPN users have no one-time password. The firmware lives in
NSM; the SSL-VPN zones and each user's one-time-password setting live on each
firewall. Without a fleet view, that is a login per client.

The same questions come back every year as a cyber-insurance questionnaire:
patched, MFA on remote access, supported hardware, security services licensed.
The licence dates sit per serial in MySonicWall, so the renewal list is a
spreadsheet. And a firewall that was registered in MySonicWall but never
onboarded to NSM is missing from every NSM report, which is the box nobody
checks.

## What this skill does about it

- `sonicwall-cli exposure "SNWLID-2024-0015"` lists every firewall NSM matched
  to the advisory and marks whether SSL-VPN is enabled on a zone whose name
  contains WAN. With no argument, `exposure` ranks the advisories that hit the
  most firewalls.
- `sonicwall-cli mfa-audit` lists admin TOTP per firewall and every local
  SSL-VPN user without a one-time password, with SAML, RADIUS, LDAP or TACACS+
  sign-in marked delegated so you check MFA at that provider.
- `sonicwall-cli posture` gives one pass, warn, fail or unknown row per firewall
  for firmware, end of support, advisories, licence expiry, admin TOTP, SSL-VPN
  exposure and SSL-VPN users without MFA, and saves each run so
  `sonicwall-cli posture changes --fleet --since 7d` shows what got worse.
- `sonicwall-cli licenses renewals --days 90` turns per-serial MySonicWall
  licence dates into one renewal list, with a replace-instead flag for hardware
  near end of support.
- `sonicwall-cli blind-spots` lists firewalls registered in MySonicWall that NSM
  cannot see.

Every one of these is read-only. A firewall the CLI could not read is reported
unknown, never pass, so a silent gap cannot look like a clean result.

## Status

Beta, awaiting live verification. The command surface is a curated subset of
SonicWall's own published API specifications (the NSM API Gateway and SonicOS
passthrough Swagger files, and the MySonicWall API), and the fleet commands are
covered by unit tests against simulated NSM and MySonicWall responses, not a
real tenant. The closed-loop receipt (a named MSP running it against their own
firewalls at a Build Session) is tracked separately and added here as
`video.md` once it exists.
