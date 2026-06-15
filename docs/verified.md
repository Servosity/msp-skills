---
layout: default
title: "Verified Connectors - MSP receipts and the live-verification wall | MSP Skills"
description: "Which MSP Skills connectors have been confirmed against a live production tenant, and which are awaiting their first MSP receipt. Every connector passes four mechanical gates before it ships; a live-verified badge means a real MSP ran it against a real tenant. Tell us yours worked in 60 seconds."
permalink: /verified/
body_class: wide
---

<span class="eyebrow">THE RECEIPTS WALL · LIVE-VERIFIED VS AWAITING</span>

# What's been verified against a real tenant

Every connector passes four mechanical gates before it ships - build, command-surface-vs-docs, claims check, and install dry-run. A **Live-verified** badge means more: a real MSP ran that connector against a real production tenant and it worked. Below is the honest state of every connector - which ones carry a live receipt, and which are still **awaiting their first MSP receipt**. Awaiting is not a defect; it's an open invitation.

{% include proof-strip.html %}

> **An honest note.** Today's live verifications are first-party - **Servosity (maintainer)** confirmed them against our own tenants. That's a real receipt, but it isn't yet a receipt from *you*. External MSP receipts are exactly what this wall wants. If you run any connector against your tenant and it works, [tell us below](#receipt) and your name goes on the wall.

{% assign verified = site.data.catalog.connectors | where: "verification", "live-verified" %}
{% assign awaiting = site.data.catalog.connectors | where_exp: "c", "c.verification != 'live-verified'" %}

## Live-verified ({{ verified | size }})

These connectors have been driven against a live production tenant.

{% for c in verified %}
**[{{ c.display_name }} →]({{ c.url }})** &nbsp; {% include verification-badge.html connector=c %}
{% endfor %}

## Awaiting their first MSP receipt ({{ awaiting | size }})

Each of these passed all four mechanical gates and is ready to run. Be the first MSP to confirm one against a live tenant - it takes about 60 seconds.

{% for c in awaiting %}
**[{{ c.display_name }} →]({{ c.url }})** &nbsp; {% include verification-badge.html connector=c %}
{% endfor %}

## Tell us it worked {#receipt}

You ran a connector against your tenant and it did the job. That's a receipt - and it's the single most useful thing you can send us. **No GitHub account needed.** Drop your email and which connector worked, or just email us. We'll add it to the wall and follow up if we have a question.

<div class="email-capture">
  <p class="ec-title">Send us a receipt</p>
  <p>Tell us which connector you verified against a live tenant. We'll put it on the wall.</p>
  <!-- TODO: swap action to Brevo/HubSpot form endpoint when creds land -->
  <form action="mailto:hello@servosity.com" method="post" enctype="text/plain">
    <input type="email" name="email" placeholder="you@yourmsp.com" aria-label="Your work email" autocomplete="email">
    <button type="submit">It worked - tell us</button>
  </form>
  <p style="margin-top:0.75rem; margin-bottom:0;">Or just email <a href="mailto:hello@servosity.com?subject=MSP%20Skills%20receipt">hello@servosity.com</a> with the connector name and your MSP. Plain text is perfect - no markdown, no issue tracker.</p>
</div>

Want to see what these connectors actually do first? [Browse every connector →](/skills/) · [Read why MSP owners use them →](/why/) · [The Trust Center →](/governance/)
