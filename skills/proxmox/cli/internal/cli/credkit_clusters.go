// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): what the
// credential kit's wiring (internal/credwire) needs from clusters.toml.

package cli

import (
	"net/url"

	"proxmox-pp-cli/internal/client"
)

// ClusterTokenSource is one clusters.toml entry as the credential kit sees it:
// the environment variables its tokens come from, and the one origin each
// token may go to ("" when the entry names no such host or the host does not
// parse, which the cluster's own validation refuses too).
type ClusterTokenSource struct {
	Name        string
	PVETokenEnv string
	PVEOrigin   string
	PBSTokenEnv string
	PBSOrigin   string
}

// ClusterTokenSources reads clusters.toml the way every command does
// (PROXMOX_CLUSTERS_FILE, else the config directory under the current --home
// override) and writes nothing. A file that is missing, unsafe or does not
// parse gives none: the commands refuse it too, so no cluster token is sent.
func ClusterTokenSources() []ClusterTokenSource {
	all, _, err := loadClusters()
	if err != nil {
		return nil
	}
	var out []ClusterTokenSource
	for _, name := range sortedClusterNames(all) {
		cl := all[name]
		src := ClusterTokenSource{Name: name, PVETokenEnv: cl.PVETokenEnv, PBSTokenEnv: cl.PBSTokenEnv}
		if base, err := clusterPVEBase(cl); err == nil && base != "" {
			if u, err := url.Parse(base); err == nil && u.Scheme != "" && u.Host != "" {
				src.PVEOrigin = u.Scheme + "://" + u.Host
			}
		}
		if cl.PBSHost != "" {
			if o, err := client.PBSOrigin(cl.PBSHost); err == nil {
				src.PBSOrigin = o
			}
		}
		out = append(out, src)
	}
	return out
}
