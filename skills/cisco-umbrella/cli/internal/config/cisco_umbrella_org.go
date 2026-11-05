// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"regexp"
	"strings"
)

// OrgEnv names the child (customer) org of a Multi-org or MSP console that
// every request is scoped to. The root --org flag exports it.
const OrgEnv = "CISCO_UMBRELLA_ORG_ID"

var orgIDPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

// ValidOrgID reports whether id is an Umbrella organization ID (digits only).
// The value travels in a request header, so nothing else is accepted.
func ValidOrgID(id string) bool { return orgIDPattern.MatchString(id) }

// ChildOrgID returns the child org the process is scoped to, or "".
func ChildOrgID() string { return strings.TrimSpace(os.Getenv(OrgEnv)) }

func init() {
	prev := StoreScopeIdentity
	// Each child org gets its own local store: synced rows from one customer
	// must never be read back as another customer's.
	StoreScopeIdentity = func(c *Config) string {
		base := ""
		if prev != nil {
			base = strings.TrimSpace(prev(c))
		}
		org := ChildOrgID()
		if org == "" {
			return base
		}
		if base == "" {
			if c.legacyStoreScopeCredential() == "" {
				return "" // logged out: nothing to scope
			}
			base = c.stableStoreScopeCredential()
		}
		return base + "|umbrella-org=" + org
	}
}
