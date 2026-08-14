# The pain point

## What breaks

An MSP stack usually contains two or three tools that never issue an API key.
The vendor's web app is the product; the API exists, but it authenticates with a
session cookie the browser marks httpOnly, or an opaque token in localStorage.

Once a month or so, that session expires. Nothing announces it. A scheduled job
starts returning empty results, or a report stops refreshing, and the failure
surfaces days later as "these numbers look wrong" rather than as an error.

Then the recovery is archaeology. Which of the several files held that token?
Was it an environment variable or a config file? Did the last person paste it
with or without the `Bearer` prefix? The capture itself takes thirty seconds.
Everything around it takes the afternoon.

## Why the obvious fixes do not work

**Ask the vendor for an API key.** Sometimes the right answer, and when it is
available `connect-tool` already handles it. But a vendor that has not shipped
one is not going to ship one because a single MSP asked.

**Read the cookie with a bookmarklet.** Not possible. httpOnly exists precisely
so page JavaScript cannot see the cookie, and it is doing its job.

**Automate the login with a headless browser.** This means storing the actual
password, and it breaks on the first multi-factor prompt or bot check. It trades
a monthly thirty-second task for a fragile system holding a stronger secret.

**Use a refresh token.** There is not one. These sessions were designed for a
human in a browser, not for a machine.

## What is actually reducible

The capture is irreducible. Everything else is not:

- Finding the secret inside the request
- Knowing which config file consumes it, and in what format
- Getting it into the OS credential store rather than a text file
- Proving the new credential actually works before walking away
- Noticing the expiry before the tool breaks instead of after

That list is the work. It is also entirely mechanical, which is why it is worth
writing down once per site as a profile and never thinking about again.

## What good looks like

A monthly re-auth is one paste and one command, ending in a live authenticated
call that either passes or fails loudly. The secret sits in the platform
credential store, not in a text file next to the code. And the warning arrives
five days before the session dies, not a week after something quietly stopped
returning data.
