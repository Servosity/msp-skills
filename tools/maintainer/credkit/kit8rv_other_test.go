// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build !unix

package credkit

import "testing"

func fifoAppends(*testing.T) {} // no FIFOs off unix
