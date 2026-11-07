// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// optInDependents are sync dependents that run only when named in
// --resources (domotz handfix per-device-sync-opt-in; the one-line check in
// syncDependentResources is re-applied by tools/apply_edits.py). Each fans out
// one request per synced device (GET /agent/{agent_id}/device/{device_id}/
// application and /variable), so a plain sync would send two requests per
// device across the account for what agent_device_application and
// agent_device_variable fetch in one request per agent.
var optInDependents = map[string]bool{
	"device_application": true,
	"device_variable":    true,
}
