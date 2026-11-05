// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it.
//
// The instance is fixed before the local store opens: config and environment
// (IMMYBOT_SUBDOMAIN, IMMYBOT_BASE_URL) pick it, root.go keys the store on it,
// and no later input may move it. `sync --path-context` writes
// Config.TemplateVars after the store is chosen, so a server-URL variable from
// it would send requests to one instance and write the answers into another's
// mirror. It refuses every server-URL variable instead.

package cli

import (
	"fmt"
	"sort"
	"strings"
)

// immyRefuseServerURLVars refuses --path-context keys that pick the instance:
// every placeholder buildURL fills from Config.TemplateVars
// (endpointTemplateVarSet; spec.json servers[0].variables.subdomain) and any
// placeholder the configured base URL carries.
func immyRefuseServerURLVars(pathContext map[string]string, baseURL string) error {
	var refused []string
	for k := range pathContext {
		if endpointTemplateVarSet[k] || strings.Contains(baseURL, "{"+k+"}") {
			refused = append(refused, k)
		}
	}
	if len(refused) == 0 {
		return nil
	}
	sort.Strings(refused)
	return fmt.Errorf("--path-context cannot set %s: it picks the ImmyBot instance, which IMMYBOT_SUBDOMAIN or IMMYBOT_BASE_URL fixes before the local store opens",
		strings.Join(refused, ", "))
}
