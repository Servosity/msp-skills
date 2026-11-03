// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import "strings"

// TokenSecret is the SECRET of a Proxmox API token, the part the credential
// kit holds. A Proxmox VE token is USER@REALM!TOKENID=SECRET and a Proxmox
// Backup Server token USER@REALM!TOKENID:SECRET, either with or without its
// PVEAPIToken= / PBSAPIToken= scheme. USER@REALM!TOKENID is the token's
// public id: Proxmox itself lists it in ordinary reads (the user of every
// task the token started, /access/users token lists, ACLs), so holding it
// would withhold those reads. A value of any other shape is returned whole,
// so it is held whole.
func TokenSecret(v string) string {
	t := strings.TrimSpace(v)
	for _, scheme := range []string{"PVEAPIToken=", "PBSAPIToken="} {
		if len(t) > len(scheme) && strings.EqualFold(t[:len(scheme)], scheme) {
			t = t[len(scheme):]
			break
		}
	}
	bang := strings.IndexByte(t, '!')
	if bang <= 0 || !strings.Contains(t[:bang], "@") {
		return v
	}
	sep := strings.IndexAny(t[bang+1:], "=:")
	if sep <= 0 {
		return v
	}
	secret := strings.TrimSpace(t[bang+1+sep+1:])
	if secret == "" {
		return v
	}
	return secret
}
