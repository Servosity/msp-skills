# wordpress skill - the MSP pain it closes

## The pain

Every routine WordPress change - swap a promo banner, fix a phone number, ship a
new landing page - means logging into wp-admin and clicking through the block
editor screen by screen. The friction is not anecdotal: the **Classic Editor**
plugin still reports **over 10 million active installs** on the WordPress.org
plugin directory, years after the block editor became the default - a standing,
measurable vote against that editor for everyday content work.

For an MSP managing a stack of client sites, that browser-clicking is unbillable
busywork. The scriptable alternative, the official **WP-CLI**, needs SSH access
to the server's shell - which many managed or hosted WordPress plans never grant -
so remote, repeatable content management falls back to the browser. On
**r/msp**, threads about client website upkeep recur for exactly this reason:
the work is manual, easy to forget, and rarely makes it onto an invoice.

## What this skill does about it

It moves that work to the terminal (or your AI agent) over the REST API the site
already exposes - no wp-admin clicking, no SSH:

- `wordpress-cli pages create --status draft` - publish or stage a landing page from HTML in one command.
- `wordpress-cli pages list --status draft` - surface every page still waiting to ship.
- `wordpress-cli media upload ./hero.png --alt-text "..."` - upload an asset and get back the media id to wire into a featured image.
- `wordpress-cli workflow archive` then `wordpress-cli search "old pricing"` - mirror the whole site locally, then find every page or post that still mentions a stale phrase.
- `wordpress-cli posts update <id> --content "..."` - correct live content in place across sites without opening a browser.

## Status

Beta. Validated against the WordPress API surface; the closed-loop receipt (a named
MSP running it live in their production tenant at a Build Session) is tracked
separately and added here as `video.md` once it exists.
