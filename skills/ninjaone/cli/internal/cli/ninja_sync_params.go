// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// ninjaSyncParamDefaults are request parameters spec.yaml documents but declares no default
// for, which a synced partition needs to answer the commands built on it. Hand-fix
// (reapply/sync_params.py renames the generated syncResourceParamDefaults).
//
//   - /v2/queries/volumes `include=bl` ("Additional information to include (bl - BitLocker
//     status)"): without it no volume carries bitLockerStatus and bitlocker-gaps reads every
//     Windows volume as NOT_REPORTED.
var ninjaSyncParamDefaults = map[string]map[string]string{
	"queries-volumes": {"include": "bl"},
}

// syncResourceParamDefaults is the rendered spec defaults plus ninjaSyncParamDefaults; a
// spec default wins on a shared key, and --param / --resource-param still override both.
func syncResourceParamDefaults(resource string) map[string]string {
	rendered := syncResourceParamDefaultsRendered(resource)
	extra := ninjaSyncParamDefaults[resource]
	if len(extra) == 0 {
		return rendered
	}
	merged := make(map[string]string, len(rendered)+len(extra))
	for k, v := range extra {
		merged[k] = v
	}
	for k, v := range rendered {
		merged[k] = v
	}
	return merged
}
