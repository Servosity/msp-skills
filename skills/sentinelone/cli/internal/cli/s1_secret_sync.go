// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). A resource whose endpoint answers with a
// credential (cliutil.IsSecretBearingPath: the agent passphrases and the
// Ranger credential details among the sync resources) is never fetched into
// the local store: sync refuses it by name before any request, every
// syncResource caller (sync, workflow archive) is refused at the fetch, and a
// live read of a secret endpoint is not written through. Called from marked
// lines in the generated sync.go and data_source.go.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"sentinelone-pp-cli/internal/cliutil"
)

// secretSyncLiveCommand names the explicit, operator-run read for each
// never-stored resource, so the refusal says where the value can be seen.
var secretSyncLiveCommand = []struct{ resource, command string }{
	{"agents-passphrases", "sentinelone-cli agents get-passphrase"},
	{"ranger", "sentinelone-cli ranger get-cred-group-details"},
}

func secretSyncResource(resource string) (string, bool) {
	name := strings.TrimSpace(resource)
	if cliutil.IsNeverStoredResource(name) {
		path := cliutil.NeverStoredResources()[strings.ReplaceAll(strings.ToLower(name), "_", "-")]
		return path, true
	}
	if path, err := syncResourcePath(name); err == nil && cliutil.IsSecretBearingPath(path) {
		return path, true
	}
	return "", false
}

func secretSyncReason(resource, path string) string {
	msg := fmt.Sprintf("%s (GET %s) returns credentials and is never stored locally", resource, path)
	name := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(resource)), "_", "-")
	for _, live := range secretSyncLiveCommand {
		if live.resource == name {
			msg += "; read it live with: " + live.command
		}
	}
	return msg
}

// refuseSecretSyncResources rejects an explicit --resources list that names a
// credential resource, before a client exists and before any request.
func refuseSecretSyncResources(requested []string) error {
	var reasons []string
	seen := map[string]bool{}
	for _, raw := range requested {
		for _, resource := range strings.Split(raw, ",") {
			resource = strings.TrimSpace(resource)
			if resource == "" || seen[resource] {
				continue
			}
			seen[resource] = true
			if path, ok := secretSyncResource(resource); ok {
				reasons = append(reasons, secretSyncReason(resource, path))
			}
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return usageErr(fmt.Errorf("sync refused, nothing was requested: %s", strings.Join(reasons, "; ")))
}

// secretSyncResourceRefusal is the fetch-time backstop for every syncResource
// caller: a credential resource is never requested for the store.
func secretSyncResourceRefusal(resource, path string) error {
	if !cliutil.IsSecretBearingPath(path) && !cliutil.IsNeverStoredResource(resource) {
		return nil
	}
	return fmt.Errorf("not synced: %s", secretSyncReason(resource, path))
}

// writeThroughUnlessSecret is writeThroughCache for every live read except a
// secret endpoint's answer, which the operator sees but the store never keeps.
func writeThroughUnlessSecret(ctx context.Context, resourceType, path string, data json.RawMessage) {
	if cliutil.IsSecretBearingPath(path) || cliutil.IsNeverStoredResource(resourceType) {
		return
	}
	writeThroughCache(ctx, resourceType, data)
}
