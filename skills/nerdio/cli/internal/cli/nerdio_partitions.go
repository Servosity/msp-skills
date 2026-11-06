// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// nerdio handfix store-partition-follows-path (Codex r1 4.34.2 class
// sync-read-partition-mismatch): every generated endpoint command names its
// command group ("account", "billing", "assisted-sales") as the store
// partition for its local reads and automatic write-through, but sync fills
// each partition from one endpoint: "account" is /account/v2/users and
// "account-v2-companies" is /account/v2/companies. The company list read
// with --data-source local returned users, and an automatic company read
// wrote companies into the users partition (and the users read the
// opportunities list wrote into "assisted-sales", which is quotes). 0.1.9
// carried the same labels.
//
// The data-source chokepoint (data_source.go: both resolve functions, the
// local reader, the write-through cache and the mutation write) now picks
// the partition by the endpoint the request path is: the path is matched to
// the most specific endpoint template the command tree declares (pp:path), and
// that template is a synced partition's path (a collection read) or one more
// {parameter} segment under it (one record read by its trailing id). A
// literal child of a synced collection (/assistedSales/v1/opportunities/
// pricingPlanCosts) is another endpoint, not a record. A path no partition is
// synced from has no local data:
// a local read says so, an automatic read is not cached and does not fall
// back to the store, so no command ever reads or writes another resource's
// rows. A mutation response is never cached: the write names only the
// group, which several resources share.

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// nerdioUnsyncedPartition is the partition of a path no sync partition
// holds; it is never read or written.
const nerdioUnsyncedPartition = "\x00nerdio:unsynced"

type nerdioSyncPath struct {
	name string
	segs []string
}

// nerdioSyncPaths is every partition sync fills, with its path segments.
var nerdioSyncPaths = sync.OnceValue(func() []nerdioSyncPath {
	names := make([]string, 0, len(syncOwnedPartitions))
	for name := range syncOwnedPartitions {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []nerdioSyncPath
	for _, name := range names {
		if p, err := syncResourcePath(name); err == nil && strings.TrimSpace(p) != "" && !nerdioTenantSecretPath(http.MethodGet, nerdioPathOnly(p)) {
			out = append(out, nerdioSyncPath{name: name, segs: nerdioPathSegments(p)})
		}
	}
	return out
})

func nerdioPathSegments(p string) []string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// nerdioPathOnly is p without its query or fragment.
func nerdioPathOnly(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		return p[:i]
	}
	return p
}

func nerdioParamSegment(s string) bool {
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
}

// nerdioSegmentsMatch reports a concrete path matching an endpoint
// template ("{id}" matches any one non-empty segment).
func nerdioSegmentsMatch(tmpl, segs []string) bool {
	if len(tmpl) != len(segs) {
		return false
	}
	for i, t := range tmpl {
		if nerdioParamSegment(t) {
			if segs[i] == "" {
				return false
			}
			continue
		}
		if t != segs[i] {
			return false
		}
	}
	return true
}

// nerdioSameTemplate reports two templates naming the same endpoint shape
// (equal literal segments, a parameter where the other has one).
func nerdioSameTemplate(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if pa, pb := nerdioParamSegment(a[i]), nerdioParamSegment(b[i]); pa != pb || (!pa && a[i] != b[i]) {
			return false
		}
	}
	return true
}

// nerdioEndpointTemplates is every endpoint path template the command
// tree declares (the pp:path annotation of each generated command).
// (A function with its own sync.Once, not a package-level OnceValue: the
// command tree's constructors reach the data-source chokepoint, so a variable
// initializer naming RootCmd would be an initialization cycle.)
func nerdioEndpointTemplates() [][]string {
	nerdioTemplates.once.Do(func() {
		seen := map[string]bool{}
		var walk func(*cobra.Command)
		walk = func(c *cobra.Command) {
			if p := strings.TrimSpace(c.Annotations["pp:path"]); p != "" && !seen[p] {
				seen[p] = true
				nerdioTemplates.list = append(nerdioTemplates.list, nerdioPathSegments(p))
			}
			for _, sub := range c.Commands() {
				walk(sub)
			}
		}
		walk(RootCmd())
	})
	return nerdioTemplates.list
}

var nerdioTemplates struct {
	once sync.Once
	list [][]string
}

// nerdioRouteTemplate is the most specific endpoint template matching a
// concrete path (the one with the most literal segments); false when none
// does or two different shapes tie.
func nerdioRouteTemplate(segs []string) ([]string, bool) {
	var best []string
	bestLiterals, tie := -1, false
	for _, tmpl := range nerdioEndpointTemplates() {
		if !nerdioSegmentsMatch(tmpl, segs) {
			continue
		}
		literals := 0
		for _, s := range tmpl {
			if !nerdioParamSegment(s) {
				literals++
			}
		}
		switch {
		case literals > bestLiterals:
			best, bestLiterals, tie = tmpl, literals, false
		case literals == bestLiterals && !nerdioSameTemplate(tmpl, best):
			tie = true
		}
	}
	return best, best != nil && !tie
}

// nerdioSyncPartitionFor is the partition whose sync path is tmpl's
// endpoint; "" when none is, nerdioUnsyncedPartition when two are.
func nerdioSyncPartitionFor(tmpl []string) string {
	found := ""
	for _, p := range nerdioSyncPaths() {
		if nerdioSameTemplate(p.segs, tmpl) {
			if found != "" && found != p.name {
				return nerdioUnsyncedPartition // two partitions claim the endpoint: neither is read
			}
			found = p.name
		}
	}
	return found
}

// nerdioStorePartition is the partition a read of path uses, and whether
// the read is of the whole collection (isList is kept for a one-record
// read). A path that is not a synced collection or one record of it gets
// nerdioUnsyncedPartition.
func nerdioStorePartition(label, path string, isList bool) (string, bool) {
	if nerdioTenantSecretPath(http.MethodGet, nerdioPathOnly(path)) {
		// nerdio handfix tenant-secrets-shown-never-stored: a tenant secret
		// (LAPS, BitLocker keys, secure variable values) is never read from or
		// written to the local store, whatever sync was told to fill.
		return nerdioUnsyncedPartition, isList
	}
	segs := nerdioPathSegments(path)
	if len(segs) == 0 {
		return nerdioUnsyncedPartition, isList
	}
	tmpl, ok := nerdioRouteTemplate(segs)
	if !ok {
		return nerdioUnsyncedPartition, isList
	}
	if name := nerdioSyncPartitionFor(tmpl); name != "" {
		return name, true
	}
	if n := len(tmpl); n > 1 && nerdioParamSegment(tmpl[n-1]) {
		if name := nerdioSyncPartitionFor(tmpl[:n-1]); name != "" {
			return name, isList
		}
	}
	_ = label // the group label names no partition by itself
	return nerdioUnsyncedPartition, isList
}

// nerdioUnsyncedLocalError is the local read of a path sync does not store.
func nerdioUnsyncedLocalError(path string) error {
	return fmt.Errorf("no local data for %s: sync does not store this endpoint, so the local store cannot answer it; use --data-source live", path)
}

// nerdioMutationWriteThrough reports whether a mutation response may be
// cached under label. Never: the label is a command group several resources
// share, and the write does not know which resource it returned.
func nerdioMutationWriteThrough(label string) bool {
	_ = label
	return false
}
