// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"acme-pp-cli/internal/config"
	"acme-pp-cli/internal/credkit"
)

// Declarations is the connector's one credkit.Declarations literal.
var Declarations = credkit.Declarations{
	SecretEnv:       []string{"ACME_TOKEN"},
	Files:           func(args []string) []string { return []string{config.Path()} },
	ConfigFields:    []string{"token", "client_secret"},
	StoredRecords:   []string{"refresh_token"},
	CredentialFlags: []string{"--token"},
	BaseURL:         credkit.BaseURL{ConfigKey: "base_url", Env: "ACME_BASE_URL"},
	Origins:         []credkit.Origin{{Source: "*", Allowed: []string{"base_url", "https://auth.example.com"}}},
	Identifiers: []credkit.Identifier{
		{Source: "env:ACME_DEBUG", Reason: "a boolean switch"},
		{Source: "config:profile", Reason: "a profile name"},
		{Source: "config:Path", Reason: "where the config file was read from"},
		{Source: "config:timeout", Reason: "a request timeout in seconds"},
	},
}
