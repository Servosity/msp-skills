# Social Card Measurement

How we know whether the social cards convert. Two surfaces, two trackers:

| Surface | Tracker | Where the data lands |
| --- | --- | --- |
| github.com repo page | Traffic API, archived daily by `.github/workflows/repo-stats.yml` | `github-repo-stats` data branch |
| Docs site (this GitHub Pages site) | GoatCounter (`goatcounter_code` in `docs/_config.yml`) | goatcounter.com dashboard + API |

GitHub Pages has no built-in analytics; the Traffic API forgets after 14 days.
Both gaps are closed by the table above.

## UTM convention

Every social post links to a docs page with:

    ?utm_source={linkedin|x|reddit|slack}&utm_medium=social&utm_campaign=<slug>-skill

Example:

    https://msp-skills.compoundingteams.com/skills/halopsa/?utm_source=linkedin&utm_medium=social&utm_campaign=halopsa-skill

GoatCounter records query params natively, so per-source and per-campaign
pageviews are queryable without extra setup.

## Posting playbook (why the og:image is not the whole story)

- LinkedIn suppresses posts with external links (large 2025-2026 studies measure
  a 19-42 percent reach penalty). Post the portrait card
  (`portrait-1080x1350.png`) as a NATIVE image, put the UTM link in the first
  comment.
- X gives link posts near-zero organic reach for non-Premium accounts. Same
  play: native image, link in reply.
- Slack and Discord unfurl the og:image but the og:title and og:description do
  most of the persuasion - keep them outcome-led (see the skill pages' front
  matter).

## Card variants and the A/B log

Card review variants live in the git-ignored `assets/social/_variants/` tree.
Per-vendor card content (accent, example command, outcome line) lives in
`docs/assets/social/cards.yaml`. The A/B loop (posts, metrics, verdicts) is run
by the maintainer's `social-ab` skill; experiment state is local, not committed.

## Setup state

- [ ] `goatcounter_code` filled in `docs/_config.yml` (maintainer signup)
- [ ] Repo secret `GHRS_GITHUB_API_TOKEN` set (token with push access)
