// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kitSourceHash is the SHA-256 that a constants table (consttable.go) is
// built for: over every other non-test Go file of the kit in name order,
// each its name, a NUL, its bytes and a NUL. vendor_credkit.py writes it
// (TestKitcpuSourceHash checks it), so a table built before an edit to the
// kit is ignored.
const kitSourceHash = "8292c8cbbcda45e5c5a1a4a02516196f8a6f848b5d3f6a7a9703cbfb86d5310d"
