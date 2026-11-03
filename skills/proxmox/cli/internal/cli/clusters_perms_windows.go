// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Windows has no POSIX mode bits; clusters.toml inherits the
// user profile's ACL.

//go:build windows

package cli

import "os"

func readClustersFile(path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304 -- path is this CLI's own clusters.toml (config dir or PROXMOX_CLUSTERS_FILE).
}
