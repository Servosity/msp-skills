// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// crowdstrike handfix store-partition-follows-path (ported from the crowdstrike
// 4.34.2 reprint, class sync-read-partition-mismatch; found here by the tests
// helper, seen/generated-partition-mismatch-seen-failing.log): every generated
// endpoint command names its command group ("devices", "policy", "mssp") as
// the store partition for its local reads and automatic write-through, but
// sync fills each partition from one endpoint: "devices" is
// /devices/combined/devices/v1. With only that partition synced, `devices
// query-hidden --data-source local` and `devices query-host-groups
// --data-source local` printed ordinary visible hosts, and an automatic read
// of any devices endpoint wrote its rows into the hosts partition.
//
// The data-source chokepoint (data_source.go: both resolve functions, the
// local reader, the write-through cache and the mutation write) now picks
// the partition by the endpoint the request path is: the path is matched to
// the most specific endpoint template the command tree declares (pp:path), and
// that template is a synced partition's path (a collection read) or one more
// {parameter} segment under it. CrowdStrike reads single records by query
// (?ids=) on /entities/ paths, which no partition syncs, so those reads are
// never answered from or cached into another resource's rows. A path no
// partition is synced from has no local data: a local read says so, an
// automatic read is not cached and does not fall back to the store. A
// mutation response is never cached: the write names only the group, which
// several resources share.

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// crowdstrikeUnsyncedPartition is the partition of a path no sync partition
// holds; it is never read or written.
const crowdstrikeUnsyncedPartition = "\x00crowdstrike:unsynced"

type crowdstrikeSyncPath struct {
	name string
	segs []string
}

// crowdstrikeSyncPaths is every partition sync fills, with its path segments.
var crowdstrikeSyncPaths = sync.OnceValue(func() []crowdstrikeSyncPath {
	names := make([]string, 0, len(syncOwnedPartitions))
	for name := range syncOwnedPartitions {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []crowdstrikeSyncPath
	for _, name := range names {
		if p, err := syncResourcePath(name); err == nil && strings.TrimSpace(p) != "" {
			out = append(out, crowdstrikeSyncPath{name: name, segs: crowdstrikePathSegments(p)})
		}
	}
	return out
})

func crowdstrikePathSegments(p string) []string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func crowdstrikeParamSegment(s string) bool {
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
}

// crowdstrikeSegmentsMatch reports a concrete path matching an endpoint
// template ("{id}" matches any one non-empty segment).
func crowdstrikeSegmentsMatch(tmpl, segs []string) bool {
	if len(tmpl) != len(segs) {
		return false
	}
	for i, t := range tmpl {
		if crowdstrikeParamSegment(t) {
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

// crowdstrikeSameTemplate reports two templates naming the same endpoint shape
// (equal literal segments, a parameter where the other has one).
func crowdstrikeSameTemplate(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if pa, pb := crowdstrikeParamSegment(a[i]), crowdstrikeParamSegment(b[i]); pa != pb || (!pa && a[i] != b[i]) {
			return false
		}
	}
	return true
}

// crowdstrikeEndpointTemplates is every endpoint path template the command
// tree declares (the pp:path annotation of each generated command).
// (A function with its own sync.Once, not a package-level OnceValue: the
// command tree's constructors reach the data-source chokepoint, so a variable
// initializer naming RootCmd would be an initialization cycle.)
func crowdstrikeEndpointTemplates() [][]string {
	crowdstrikeTemplates.once.Do(func() {
		seen := map[string]bool{}
		var walk func(*cobra.Command)
		walk = func(c *cobra.Command) {
			if p := strings.TrimSpace(c.Annotations["pp:path"]); p != "" && !seen[p] {
				seen[p] = true
				crowdstrikeTemplates.list = append(crowdstrikeTemplates.list, crowdstrikePathSegments(p))
			}
			for _, sub := range c.Commands() {
				walk(sub)
			}
		}
		walk(RootCmd())
	})
	return crowdstrikeTemplates.list
}

var crowdstrikeTemplates struct {
	once sync.Once
	list [][]string
}

// crowdstrikeRouteTemplate is the most specific endpoint template matching a
// concrete path (the one with the most literal segments); false when none
// does or two different shapes tie.
func crowdstrikeRouteTemplate(segs []string) ([]string, bool) {
	var best []string
	bestLiterals, tie := -1, false
	for _, tmpl := range crowdstrikeEndpointTemplates() {
		if !crowdstrikeSegmentsMatch(tmpl, segs) {
			continue
		}
		literals := 0
		for _, s := range tmpl {
			if !crowdstrikeParamSegment(s) {
				literals++
			}
		}
		switch {
		case literals > bestLiterals:
			best, bestLiterals, tie = tmpl, literals, false
		case literals == bestLiterals && !crowdstrikeSameTemplate(tmpl, best):
			tie = true
		}
	}
	return best, best != nil && !tie
}

// crowdstrikeSyncPartitionFor is the partition whose sync path is tmpl's
// endpoint; "" when none is, crowdstrikeUnsyncedPartition when two are.
func crowdstrikeSyncPartitionFor(tmpl []string) string {
	found := ""
	for _, p := range crowdstrikeSyncPaths() {
		if crowdstrikeSameTemplate(p.segs, tmpl) {
			if found != "" && found != p.name {
				return crowdstrikeUnsyncedPartition // two partitions claim the endpoint: neither is read
			}
			found = p.name
		}
	}
	return found
}

// crowdstrikeStorePartition is the partition a read of path uses, and whether
// the read is of the whole collection (isList is kept for a one-record
// read). A path that is not a synced collection or one record of it gets
// crowdstrikeUnsyncedPartition.
func crowdstrikeStorePartition(label, path string, isList bool) (string, bool) {
	segs := crowdstrikePathSegments(path)
	if len(segs) == 0 {
		return crowdstrikeUnsyncedPartition, isList
	}
	tmpl, ok := crowdstrikeRouteTemplate(segs)
	if !ok {
		return crowdstrikeUnsyncedPartition, isList
	}
	if name := crowdstrikeSyncPartitionFor(tmpl); name != "" {
		return name, true
	}
	if n := len(tmpl); n > 1 && crowdstrikeParamSegment(tmpl[n-1]) {
		if name := crowdstrikeSyncPartitionFor(tmpl[:n-1]); name != "" {
			return name, isList
		}
	}
	_ = label // the group label names no partition by itself
	return crowdstrikeUnsyncedPartition, isList
}

// crowdstrikeUnsyncedLocalError is the local read of a path sync does not store.
func crowdstrikeUnsyncedLocalError(path string) error {
	return fmt.Errorf("no local data for %s: sync does not store this endpoint, so the local store cannot answer it; use --data-source live", path)
}

// crowdstrikeMutationWriteThrough reports whether a mutation response may be
// cached under label. Never: the label is a command group several resources
// share, and the write does not know which resource it returned.
func crowdstrikeMutationWriteThrough(label string) bool {
	_ = label
	return false
}
