# Preserve the Action1 installer discovery contract

The software repository package and version APIs already support reads and
configuration updates. Preserve the agent-facing installer summaries, keywords,
README recipe, SKILL guidance, search regression, and handfixes.json entry.

An organization ID selects that organization's view. The all selector requires
the repository permission at Enterprise scope. Read access through an
organization view does not establish Enterprise write access; preserve HTTP
403 explanations and never grant permissions or change scope automatically.
Rediscover the package scope before writing rather than guessing it.

Package GET with fields: versions discovers version object IDs. The versionId
parameter is the returned id, not the displayed version number. Package PATCH
changes package metadata; version PATCH changes deployment configuration.
Put edited version properties directly in MCP params. The CLI accepts the
equivalent object with --body-json or --stdin. Never replay a full GET response.

Version configuration requires manage_software_repository. approval_status
requires approve_updates and EULA_accepted requires accept_eula. Script actions
also require the appropriate Use Scripts scope. Built-in versions are read-only.

Build search keywords from the same summaries. Searching for installer must
return the package GET and version PATCH with their ID, field, and permission
guidance. Read back an authorized update; this is not endpoint installation
or deployment verification. Binary upload support is a separate transport task.
