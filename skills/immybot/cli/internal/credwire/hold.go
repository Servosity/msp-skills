// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"slices"
	"strings"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/config"
	"immybot-pp-cli/internal/credkit"
)

// The kit's list of each, as code here may read them: Declarations stays a literal
// only Start reads (TestTheHoldListsMatchTheDeclarations keeps these equal).
var (
	requestCredentialFields = []string{"client_secret", "refresh_token", "password", "localAdminPassword", "packagePassword", "wirelessKey", "customAppRegSecret"}
	credentialFlags         = []string{"--client-secret", "--password", "--ppkg-options-local-admin-password", "--ppkg-options-package-password", "--ppkg-options-wireless-key", "--auth-details-custom-app-reg-secret"}
)

// HoldStartup holds what credkit.Start cannot name: every value of every file a
// run could read, not only its declared keys. Both mains call it right after
// Start, because a flag that fails to parse echoes its text and no command hook
// runs after that.
func HoldStartup(args []string) {
	restoreBaseURLEnv()
	config.HoldProcessCredentials("")
	for _, path := range credkit.FlagValues(args, "--config") {
		config.HoldProcessCredentials(path)
	}
	for _, home := range credkit.FlagValues(args, "--home") {
		if restore, err := cliutil.SetHomeOverride(home); err == nil {
			config.HoldProcessCredentials("")
			restore()
		}
	}
}

// HoldRequest holds every declared credential field of a request as soon as it
// exists, wherever it came from (a flag, an import file, an MCP tool; a --stdin
// body is held whole from startup), so a --dry-run preview that never reaches the
// outbound guard holds it too. A body that is not JSON is never previewed, and the
// guard reads it on the way out.
func HoldRequest(body []byte, params map[string]string) {
	for name, value := range params {
		if field, ok := credentialField(name); ok {
			cliutil.HoldValues("field:"+field, value)
		}
	}
	cliutil.JSONLeaves(body, func(keys []string, value string) {
		for _, key := range keys {
			if field, ok := credentialField(key); ok {
				cliutil.HoldValues("field:"+field, value)
				return
			}
		}
	})
}

// credentialField is the declared field a request name is, by its last dotted
// segment, as the kit compares them.
func credentialField(name string) (string, bool) {
	last := strings.ToLower(name[strings.LastIndexByte(name, '.')+1:])
	return last, slices.ContainsFunc(requestCredentialFields, func(f string) bool { return strings.EqualFold(f, last) })
}

// CredentialFlag reports a flag name (no dashes) the kit holds as a credential.
func CredentialFlag(name string) bool {
	return slices.ContainsFunc(credentialFlags, func(f string) bool { return strings.EqualFold(f, "--"+name) })
}
