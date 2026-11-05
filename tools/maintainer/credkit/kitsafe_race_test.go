// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build race

package credkit

// raceBuild: the race detector slows the matcher about tenfold, so the
// largest ordinary units run without it only.
const raceBuild = true
