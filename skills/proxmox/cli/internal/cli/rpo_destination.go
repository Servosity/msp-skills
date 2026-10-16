// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: where a backup job writes, so rpo judges each job's runs
// against the restore points at its own destination.

package cli

import "strings"

// destinationKey names where a job writes: for a PBS storage the PBS server,
// datastore and namespace (two storage ids for one datastore are one
// destination; one datastore name on two servers is two), else the storage.
func destinationKey(target string, ref *pbsStorageRef) string {
	if ref != nil {
		return "pbs:" + strings.ToLower(ref.server) + "/" + ref.datastore + "/" + ref.namespace
	}
	return "pve:" + target
}

// onDestination: rp is a backup at that destination (a PBS snapshot by its
// server, datastore and namespace, read through either API; any other
// archive by its storage id, on whichever node).
func onDestination(rp restorePoint, key string) bool {
	if strings.HasPrefix(key, "pbs:") {
		server, _, ok := strings.Cut(strings.TrimPrefix(rp.canonical, "pbs:"), "/")
		return ok && strings.HasPrefix(rp.canonical, "pbs:") && key == "pbs:"+strings.ToLower(server)+"/"+rp.pbsStore+"/"+rp.pbsNS
	}
	return strings.HasPrefix(rp.canonical, key+"@")
}
