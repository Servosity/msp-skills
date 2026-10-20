// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Scheduled-policy resolution (why-blocked verdicts, allow's assignment
// fallback) loads IANA zones such as America/New_York with time.LoadLocation.
// Windows has no system zoneinfo, so without Go installed or ZONEINFO set a
// valid schedule would stay unresolved. Embedding the database (about 450 KB)
// makes every zone resolvable on every platform; a system database, when
// present, is still preferred.
import _ "time/tzdata"
