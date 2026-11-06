// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//go:build !windows

package cli

// ensureSpace is a no-op off Windows: the delivery drive is a Windows
// volume, and a failed write reports the problem anyway.
func ensureSpace(string, int64) error { return nil }
