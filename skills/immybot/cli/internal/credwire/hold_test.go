// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"slices"
	"strings"
	"testing"
)

func TestTheHoldListsMatchTheDeclarations(t *testing.T) {
	if !slices.Equal(requestCredentialFields, Declarations.CredentialFields) {
		t.Errorf("requestCredentialFields = %v, Declarations.CredentialFields = %v", requestCredentialFields, Declarations.CredentialFields)
	}
	if !slices.Equal(credentialFlags, Declarations.CredentialFlags) {
		t.Errorf("credentialFlags = %v, Declarations.CredentialFlags = %v", credentialFlags, Declarations.CredentialFlags)
	}
}

// An instance's credentials go to that instance: the kit reads the origin from
// the selected config (TestTheSubdomainsInstanceIsTheAllowedOrigin), so no origin
// here may cover every instance.
func TestNoOriginCoversAnotherInstance(t *testing.T) {
	for _, o := range Declarations.Origins {
		for _, allowed := range o.Allowed {
			if strings.Contains(allowed, "*") {
				t.Errorf("source %q allows %q", o.Source, allowed)
			}
		}
	}
}
