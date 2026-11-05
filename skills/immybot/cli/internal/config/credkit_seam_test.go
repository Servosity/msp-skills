// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "immybot-pp-cli/internal/cliutil"

func init() { cliutil.HoldValues = func(string, ...string) {} }
