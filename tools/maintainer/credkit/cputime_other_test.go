// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !unix

package credkit

func cpuTime() float64 { return 0 }
