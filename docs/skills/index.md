---
layout: default
title: "MSP Connectors - Every PSA, RMM, Backup, and M365 Tool | MSP Skills"
description: "Browse every MSP Skills connector - free, open source MCP servers and Claude Code Skills for PSA, RMM, backup, security, billing, and M365 tools. Grouped by category, each runs on your own machine and works with Claude, ChatGPT, Codex, and Copilot."
permalink: /skills/
body_class: wide
---

<span class="eyebrow">{{ site.data.catalog.count }} FREE CONNECTORS · GROUPED BY CATEGORY</span>

# Find your MSP tool

{{ site.data.catalog.count }} connectors are live today, grouped by category below - PSA, RMM, backup, security, billing, documentation, and M365. Each one is a free, open source MCP server and Claude Code Skill that runs on your own machine and works with the AI you already use. Click any connector for its install page, and look for the badge that tells you whether a real MSP has confirmed it against a live tenant yet.

[New to the term? What is an MCP server? →](/what-is-an-mcp-server/) &nbsp; [Why add this if my vendor ships one? →](/#why-this-one) &nbsp; [The Trust Center →](/governance/)

{% assign grouped = site.data.catalog.connectors | group_by: "category" | sort: "name" %}
{% for group in grouped %}
## {{ group.name }}

| Connector | What it does | Status |
| --- | --- | --- |
{% for c in group.items -%}
| [{{ c.display_name }} →]({{ c.url }}) | {{ c.tagline }} | {% if c.verification == "live-verified" %}Live-verified{% else %}Awaiting MSP receipt{% endif %} |
{% endfor %}

{% endfor %}

## Don't see your tool?

Tell us what to build next - name the PSA, RMM, backup, security, or M365 tool and we'll add it to the pipeline. **[Request a connector →](/requesting-a-skill/)** or open a [Build Session](https://compoundingteams.com/build-sessions) request to watch it built live against a real tenant.

Once you've picked one, [install it in about 60 seconds →](/#install-in-60-seconds). Want your AI to choose for you? [Let the concierge recommend connectors for your stack →](/#let-your-ai-pick-for-you).
