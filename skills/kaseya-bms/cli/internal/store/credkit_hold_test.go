// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "kaseya-bms-pp-cli/internal/credkit"

// holdForSplitTest holds v as the kit holds the login password.
func holdForSplitTest(v string) { credkit.Hold("env:KASEYA_BMS_PASSWORD", v) }
