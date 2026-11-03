// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

//go:build credmatch

package main

import "credkit"

func credkitDecl() decl {
	return decl{SecretEnv: []string{"ACME_API_KEY", "ACME_CLIENT_ID"},
		ResponseFields: []string{"x-request-id", "password"},
		Identifiers:    []credkit.Identifier{{Source: "env:ACME_CLIENT_ID", Reason: "a public tenant identifier"}}}
}
