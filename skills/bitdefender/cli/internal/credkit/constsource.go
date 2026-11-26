// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kitSourceHash is the SHA-256 that a constants table (consttable.go) is
// built for: over every other non-test Go file of the kit in name order,
// each its name, a NUL, its bytes and a NUL. vendor_credkit.py writes it
// (TestKitcpuSourceHash checks it), so a table built before an edit to the
// kit is ignored.
const kitSourceHash = "871849f59e453fc81f7bade46e81b2d04e17ca098e6b4e771369e6719c941ce6"
