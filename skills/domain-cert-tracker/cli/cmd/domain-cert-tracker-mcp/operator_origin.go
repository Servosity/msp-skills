// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import "domain-cert-tracker-pp-cli/internal/cli"

// Tool calls never widen the outbound guard to a config-file base_url.
func init() { cli.MarkMCPServer() }
