// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-bms handfix local-partition-follows-the-path (Codex review r1 class
// "wrong local partition"): the generator names one store partition per
// command group, so every account command (sites, devices, users, variables,
// components, alerts, the account itself) read from and cached into
// `account`, the partition sync fills with /v2/account/sites and every fleet
// analytic reads as sites. A default (auto) live read cached its records
// there and a --data-source local read of devices returned the synced sites.
//
// One rule for every generated read, cache and write-back: a sync partition
// serves and receives only the records of its own sync path. A read whose
// path is a sync path uses that partition; a read that names a sync partition
// of another path has no local partition (no local read, no cache); a
// partition no sync path owns stays the generated name. A mutation response
// never enters a sync partition (it carries no path to prove its shape).

import (
	"fmt"
	"strings"
)

// kbmsLocalPartition returns the partition a generated read of path may
// serve from and cache into ("" for none).
func kbmsLocalPartition(resourceType, path string) string {
	p := kbmsPathSegments(path)
	for name := range syncOwnedPartitions {
		sp, err := syncResourcePath(name)
		if err == nil && kbmsPathMatches(kbmsPathSegments(sp), p) {
			return name
		}
	}
	if syncOwnsPartition(resourceType) {
		return ""
	}
	return resourceType
}

// kbmsNoLocalPartition is the refusal of a local read with no partition.
func kbmsNoLocalPartition(path string) error {
	return fmt.Errorf("no local data for %s: sync does not keep this endpoint; use --data-source live", path)
}

func kbmsPathSegments(path string) []string {
	path = strings.SplitN(path, "?", 2)[0]
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// kbmsPathMatches reports a request path that is the sync path template (a
// {placeholder} segment matches any one non-empty segment).
func kbmsPathMatches(template, path []string) bool {
	if len(template) == 0 || len(template) != len(path) {
		return false
	}
	for i, seg := range template {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if path[i] == "" {
				return false
			}
			continue
		}
		if seg != path[i] {
			return false
		}
	}
	return true
}
