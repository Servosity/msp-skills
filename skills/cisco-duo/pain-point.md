# cisco-duo skill - the MSP pain it closes

## The pain

"I'm locked out of Duo" is a routine MSP help-desk ticket, and it is never one lookup.
Duo locks a user out when failed authentications pass the account's "Failed attempts"
threshold, or when they stay unenrolled too long
([Duo's user administration docs](https://duo.com/docs/administration-users)), and the
fix depends on why: a user locked out for failed attempts needs an unlock, one
who lost their phone needs a new one attached and activated, one in a Disabled group
stays blocked whatever their own status says, and one locked out by Trust Monitor should
not be unlocked at all without a closer look. In the Admin Panel those facts live on
separate pages: the user, their groups, their phones, their bypass codes, and the
authentication log.

For an MSP the same ticket starts with a harder question: *which customer is this
caller in?* Duo's MSP program runs every customer as a separate subaccount with its own
API host. MSPs on the Duo Community forum ask whether each technician has to be set up
separately for admin access in every client account
([Duo Community: "As a MSP, does my team need to setup authentication for admin rights
for each client?"](https://community.duo.com/t/as-a-msp-does-my-team-need-to-setup-authentication-for-admin-rights-for-each-client/9732)),
and the per-customer split runs through every cross-customer question after that: who
can skip MFA across our book, who never enrolled, which customer is about to hit its
user limit.

The quiet risk is the bypass code issued during the last emergency. A code with no
expiry or unlimited reuse is a standing hole in MFA, and nothing in the console lists
them across a tenant with the admin who issued each one.

## What this skill does about it

- **`lockout triage <user>`** - one read-only call returns the user's effective status
  (including a group that overrides it), lockout reason, phones, outstanding bypass
  codes, the last 24 hours of failures, and the exact next command to run.
- **`lockout fix <user> --bypass`** - unlocks a locked-out user and issues one
  single-use bypass code that expires in an hour, or re-sends Duo Mobile activation;
  it prints the plan and changes nothing until `--apply`.
- **`phone-swap <user> --number <+E.164>`** - moves a user to a new phone number and
  texts the activation link, then retires the old phone on a second run once the new one
  is activated.
- **`fleet find-user`, `fleet risk`, `fleet summary`** - answer "which customer is this
  person in", "who can skip MFA across every customer", and "who is near their user
  limit or out of telephony credits" in one call each.
- **`audit bypass`** - lists every user in a tenant who can skip MFA, with standing
  bypass codes and the admin who issued each.

## Status

Beta. Validated against the Cisco Duo Admin API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.

