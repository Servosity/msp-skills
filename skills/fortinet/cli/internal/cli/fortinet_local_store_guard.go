// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Guards that keep the local store from answering a question it cannot.
//
// VDOM: the store is not VDOM-scoped. sync reads every table without a vdom
// parameter (the API user's default VDOM) and rows are keyed by name, so an
// object cached from one VDOM would answer for another: client VDOMs on one
// FortiGate commonly reuse object names. A read scoped to an explicit --vdom
// is therefore never answered from the store and never written into it, and
// sync accepts only property-only parameters through its free-form parameter
// flags.
//
// Shared tables: every generated monitor command reads and writes the one
// resource type "monitor", and every settings command the one type
// "settings", so a cached row from one endpoint (certificates, FortiAnalyzer
// settings) would answer another (HA checksums, syslog settings). Those
// commands are never answered from the store and never written into it.
//
// Device: the generated store key hashes only the credential, so pointing
// FORTINET_BASE_URL at another FortiGate with the same token configured (or
// that FortiGate being unreachable, which falls back to the store) answered
// with the first device's rows. The key now also carries the normalized API
// origin, and an older credential-only store is never adopted into a
// device's key (helpers.go passes no legacy credential), because nothing
// records which device filled it.

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"fortinet-pp-cli/internal/config"
)

func init() {
	config.StoreScopeIdentity = storeScopeIdentity
}

// storeScopeIdentity keys the local store by API origin and credential.
func storeScopeIdentity(c *config.Config) string {
	cred := strings.TrimSpace(c.StoreScopeLegacyCredential())
	if cred == "" {
		return ""
	}
	return "origin=" + storeOrigin(c.BaseURL) + "\n" + cred
}

// storeOrigin normalizes an API base URL: scheme and host lowercased, no
// trailing slash. An unparseable value is used as written.
func storeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
}

// vdomScopedRead reports whether a read targets an explicit VDOM.
func vdomScopedRead(params map[string]string) bool {
	return strings.TrimSpace(params["vdom"]) != ""
}

// sharedLocalTables are the resource types several generated commands with
// different endpoints read and write. TestSharedLocalTablesCoverEveryCommand
// fails when a regenerated command adds another one.
var sharedLocalTables = map[string]bool{"monitor": true, "settings": true}

// localStoreRefused is the closed failure for a local read the store cannot
// answer correctly, explicit (--data-source local) or as the auto-mode network
// fallback. It is a localQueryUnsupportedError, so the fallback reports what
// local data could not apply instead of serving another VDOM's or another
// endpoint's rows.
func localStoreRefused(resourceType string, params map[string]string) error {
	if vdomScopedRead(params) {
		return newLocalQueryUnsupportedError([]string{"vdom"})
	}
	if sharedLocalTables[resourceType] {
		return newLocalQueryUnsupportedError([]string{"this endpoint (" + resourceType + " commands share one local table)"})
	}
	return nil
}

// localStoreWritable reports whether a live result may be written through to
// the store: never for a VDOM-scoped read or a shared table.
func localStoreWritable(resourceType string, params map[string]string) bool {
	return !vdomScopedRead(params) && !sharedLocalTables[resourceType]
}

// syncPropertyOnlyParams are the FortiOS cmdb query parameters that only add
// or hide properties and never change which rows a pass returns: datasource
// (linked-object detail), with_meta (object metadata) and skip (hide skipped
// properties). Everything else is refused below, because a parameter that
// narrows rows (filter, key and pattern, start and count, format without the
// primary key, exclude-default-values, action) would make sync prune every
// cached row the pass did not return.
var syncPropertyOnlyParams = map[string]bool{"datasource": true, "with_meta": true, "skip": true}

// syncScopeRefused rejects a sync whose --param, --global-param or
// --resource-param is not property-only. vdom gets its own reason: the rows
// would land in the default-VDOM store.
func syncScopeRefused(p *syncUserParams) error {
	if p == nil {
		return nil
	}
	type source struct {
		flag string
		m    map[string]string
	}
	sources := []source{{"--param ", p.flatGlobal}, {"--global-param ", p.trueGlobal}}
	resources := make([]string, 0, len(p.perResource))
	for r := range p.perResource {
		resources = append(resources, r)
	}
	sort.Strings(resources)
	for _, r := range resources {
		sources = append(sources, source{"--resource-param " + r + ":", p.perResource[r]})
	}
	for _, src := range sources {
		keys := make([]string, 0, len(src.m))
		for k := range src.m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			canon := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
			switch {
			case canon == "vdom":
				return fmt.Errorf("%s%s is not supported: the local store holds only the default VDOM (sync reads without a vdom parameter), so VDOM-scoped rows would be mixed into it. Read another VDOM live with the --vdom flag of the list and get commands", src.flag, k)
			case !syncPropertyOnlyParams[canon]:
				return fmt.Errorf("%s%s is not supported: sync stores whole tables and prunes cached rows a complete pass did not return, so it accepts only parameters that never change which rows come back (datasource, skip, with_meta). Filter or page at read time instead (the --filter flag of list commands, or search and sql)", src.flag, k)
			}
		}
	}
	return nil
}
