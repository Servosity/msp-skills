// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): how a mirrored
// MCP tool's child gets the environment's API token. The credential kit starts
// no child with a credential in its argv or environment, and strips declared
// secret variables from the one it inherits, so the MCP server writes the token
// to the child's stdin behind a flag the kit declares a stdin credential.

package cliutil

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
)

// CredentialStdinFlag is the hidden flag that makes the program read the API
// token from stdin. Declarations.StdinFlags names it, so credkit.Start holds
// what it reads before anything is written.
const CredentialStdinFlag = "--credentials-stdin"

// SecretEnvNames are Declarations.SecretEnv, the variables a child never
// inherits (TestSecretEnvNamesMatchDeclarations keeps the two equal).
var SecretEnvNames = []string{"SERVOSITY_MSP_TOKEN", "SERVOSITY_MSP_MCP_HTTP_TOKEN"}

// IsSecretEnvEntry reports whether a NAME=value entry sets a secret variable.
// The name is compared without case, as Windows reads it.
func IsSecretEnvEntry(entry string) bool {
	name, _, _ := strings.Cut(entry, "=")
	return slices.ContainsFunc(SecretEnvNames, func(s string) bool { return strings.EqualFold(s, name) })
}

// ReadCredentialStdin applies the token a parent wrote to stdin when the
// command line asks for it: the rest of the program reads it from the
// environment, as it would from any install. A terminal has nothing to read.
func ReadCredentialStdin() error {
	if !slices.Contains(os.Args[1:], CredentialStdinFlag) {
		return nil
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	token, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if token = strings.TrimSpace(token); token != "" {
		return os.Setenv("SERVOSITY_MSP_TOKEN", token)
	}
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("reading the token from stdin: %w", err)
	}
	return nil
}
