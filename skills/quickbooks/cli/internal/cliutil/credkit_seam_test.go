// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "quickbooks-pp-cli/internal/credkit"

// A hold lasts until the process exits; see HoldCredential.
func init() { HoldCredential = func(credkit.Source, ...string) {} }
