# Pull request

Thanks for sending this. Delete any section that does not apply - none of it is
required to open the PR, and we would rather see the work than a filled-in form.

## What this changes

<!-- One sentence. What does this add or fix? -->

## What kind of change is it?

<!-- Tick one. -->

- [ ] A **connector** - a Go CLI plus MCP server for a vendor's API
- [ ] A **markdown-only skill** - instructions, maybe some scripts, no compiled binary
      (remember `"markdown_only": true` in `tools/maintainer/skills.json`)
- [ ] A **fix** to a connector or skill that already exists
- [ ] Docs, tooling, or something else

## Did you run it against a real tenant?

<!-- If yes, say which commands and what came back. Read-only is fine and normal.
     If no, say so - we will not hold it against you, it just tells us what still
     needs proving. Please do not paste real client names or ticket contents. -->

## Anything you could not do from your side?

<!-- Expected and fine. A maintainer finishes these after merge:
       - social preview images and the demo video (internal toolchain)
       - the live-verified badge (only a real MSP's report flips it)
       - generated files you could not regenerate locally
     Tell us which ones and we will pick them up. -->

## Anything that looks like our bug?

<!-- A check failing for a reason that is not about your code, a tool that
     crashed on your OS, a doc that was wrong. Please say so. That is a real
     contribution and we want it. -->

---

<details>
<summary>What the automated checks will look for (all explained in CONTRIBUTING.md)</summary>

- Every commit signed off (`git commit -s`) - one line saying the code is yours
  to give. If you forget, the check prints the command that fixes it.
- `SKILL.md` frontmatter: `name`, `description`, `allowed-tools`, `author`,
  `license`, `vendor`.
- `README.md` opens with the non-affiliation banner (third-party vendors).
- Vendor names used descriptively only - no logos, no "Official" / "Certified" /
  "Partner".
- No em-dashes; no personal paths, emails, or API keys.
- `install.sh` + `install.ps1` for a connector. Markdown-only skills do not need
  them.

If `security-gate` flags something, note that its policy is read from `main`, so
you **cannot** approve an exception from your own branch. Fix it if it is real
(usually a dependency bump); say so in the PR if you think it is a false
positive, and a maintainer will handle it.

</details>
