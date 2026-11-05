// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (NOT generated; handfixes.json read-only-tools-change-nothing):
// an MCP tool annotated read-only skips the confirm gate, so its CLI child may
// change nothing on this machine. After RefuseWrites, the store opens
// read-only, and the journal and the audit and teach logs are not written.

import "sync/atomic"

var writesRefused atomic.Bool

// RefuseWrites turns local writes off for the rest of this process.
func RefuseWrites() { writesRefused.Store(true) }

// WritesRefused reports whether RefuseWrites was called.
func WritesRefused() bool { return writesRefused.Load() }
