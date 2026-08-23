// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Installs Avanan request signing on every client the CLI constructs.
//
// Avanan's legacy scheme requires a freshly computed signature on every
// request, so it cannot be expressed as the static header set the spec's
// apiKey scheme produces. Registering a client hook applies the signing
// transport to generated endpoint commands and hand-written commands alike,
// without editing any generated file.

package cli

import "avanan-pp-cli/internal/client"

func init() {
	registerClientHook(client.InstallAvananAuth)
}
