// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: which reads the response cache never stores (free-text notes).
package client

import "testing"

func TestNoStoreResponsePaths(t *testing.T) {
	match := []string{"/nodes/pve1/config", "nodes/pve1/qemu/100/config", "/nodes/pve1/lxc/102/config", "/nodes/pve1/qemu/100/pending", "/nodes/pve1/qemu/100/snapshot", "/nodes/pve1/qemu/100/snapshot/s1/config", "/nodes/pve1/storage/local/content", "/nodes/pve1/storage/local/content/local:backup/x.vma.zst", "/nodes/pve1/storage/local/content/local%3Abackup%2Fx", "/admin/datastore/main/snapshots", "/admin/datastore/main/notes", "/admin/datastore/main/group-notes", "/pools", "/pools/p1", "/nodes/pve1/qemu/100/config?current=1"}
	keep := []string{"/nodes", "/nodes/pve1/storage", "/cluster/resources", "/nodes/pve1/qemu/100/status/current", "/nodes/pve1/qemu", "/admin/datastore", "/admin/datastore/main/groups", "/nodes/pve1/qemu/100/snapshot/s1", "/nodes/pve1/tasks"}
	for _, p := range match {
		if !noStoreResponse(p) {
			t.Errorf("should not cache %s", p)
		}
	}
	for _, p := range keep {
		if noStoreResponse(p) {
			t.Errorf("should cache %s", p)
		}
	}
}
