// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// hand-fix (handfixes.json client-profile-not-enforced, ported from
// threatdown): no WatchGuard platform adapter is registered, so
// --client-profile / PRINTING_PRESS_CLIENT_PROFILE gates nothing (a
// nonexistent profile name is accepted and the call reaches the configured
// account). The flag help must not promise a tenant-gated profile.
func TestClientProfileFlagHelpIsHonest(t *testing.T) {
	if registeredPlatformSource != nil {
		t.Skip("a platform source is registered; the profile is enforced and this help must change back")
	}
	flag := RootCmd().PersistentFlags().Lookup("client-profile")
	if flag == nil {
		t.Fatal("--client-profile flag missing")
	}
	if !strings.HasPrefix(flag.Usage, "Not enforced by this connector") || strings.Contains(flag.Usage, "Select the tenant-gated client profile") {
		t.Fatalf("--client-profile help promises gating that does not exist: %q", flag.Usage)
	}
}
