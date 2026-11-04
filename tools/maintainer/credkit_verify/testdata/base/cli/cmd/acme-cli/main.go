// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"acme-pp-cli/internal/cli"
	"acme-pp-cli/internal/credkit"
)

func main() {
	credkit.Start(cli.Declarations)
	credkit.Exit(cli.Execute())
}
