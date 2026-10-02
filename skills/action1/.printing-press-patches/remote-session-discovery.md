# Preserve the Action1 remote assistance request contract

Remote desktop is already implemented by the remote-session POST, GET and
monitor PATCH endpoints. Preserve discoverability of the mandatory POST params
orgId, endpointId and connection_type with the supported value assistance.
The CLI example must use assistance rather than a generated placeholder.

The search result for session status must explain that the returned session id
is passed as sessionId, connected must become yes, and remote_session is the
browser connection URL. A successfully requested session is not a verified
desktop connection. Poll with a bounded wait and stop on errors.

Remote Connect must be granted to the API credential's role in the endpoint's
scope. Script execution permission does not include it. Preserve the 403 and
its permission explanation; do not substitute another access path or grant
privileges automatically. The README recipe and handfixes.json entry pin this
Action1-specific behavior from the vendor's API reference.

Build the search keywords from the same updated summaries. Searching for
assistance must find the session POST, and connected or browser must find the
POST and status GET. Preserve the search regression test alongside the metadata.
Keep the command constructor documented and the lint schema set to version 2;
the generated formatters configuration already uses that schema.

Both session search summaries must state the one-minute polling limit, stop on
API errors, keep the connection URL private, and avoid creating another session
automatically after a timeout. Verify the intended desktop before interaction.
