// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "db-flag-help-names-scoped-mirror".
package cli

// esetDBDefault is the one description of where --db points by default. The
// generated help said "resolved data directory data.db" (and teach playbook
// "standard cache location"), but the mirror is credential-scoped:
// defaultDBPath picks data-<hash>.db in the data directory once a credential
// is set, data.db only before that.
const esetDBDefault = "default: this CLI's scoped mirror in its data directory, data-<hash>.db, or data.db before a credential is set; 'doctor' prints the path"

// esetDBFlagUsage is the --db usage for every command that reads or writes the
// local mirror.
const esetDBFlagUsage = "Path to the local SQLite mirror (" + esetDBDefault + ")"
