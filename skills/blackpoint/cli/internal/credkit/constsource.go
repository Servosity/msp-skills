// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kitSourceHash is the SHA-256 that a constants table (consttable.go) is
// built for: over every other non-test Go file of the kit in name order,
// each its name, a NUL, its bytes and a NUL. vendor_credkit.py writes it
// (TestKitcpuSourceHash checks it), so a table built before an edit to the
// kit is ignored.
const kitSourceHash = "3387d6378617937547b070c5017cccc726a6b426a19ccfc5331ecb35a677776c"
