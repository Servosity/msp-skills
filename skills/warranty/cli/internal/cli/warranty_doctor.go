// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-doctor): doctor's per-vendor section.
// Each vendor is optional, so a missing one is INFO and never fails the
// others; only "no vendor configured at all" is a WARN.

package cli

import (
	"context"
	"fmt"
	"strings"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/config"
	"warranty-pp-cli/internal/warranty"
)

// doctorProbeTag is a service tag no device has. Dell's entitlement endpoint is
// the only read with Dell's credential and it needs a tag, so the probe spends
// one call on this one; only the status matters, never the body.
const doctorProbeTag = "0000000"

// doctorProbeDell writes the credentials verdict from one authenticated read,
// and corrects the api row when that read proves it optimistic (issue #282).
func doctorProbeDell(ctx context.Context, c *client.Client, report map[string]any) {
	path := "/asset-entitlements?servicetags=" + doctorProbeTag
	status, err := c.ProbeGet(ctx, path)
	switch {
	case err == nil:
		report["credentials"] = "valid (verified with GET /asset-entitlements)"
	case status == 401:
		report["credentials"] = "ERROR rejected (HTTP 401 from /asset-entitlements): the Dell credential is invalid or expired."
	case status == 403:
		report["credentials"] = "scope-limited (HTTP 403 from /asset-entitlements): the Dell credential is accepted but lacks the Warranty API."
	case status == 404:
		report["credentials"] = "ERROR not verified (HTTP 404 from /asset-entitlements): the endpoint does not exist at this base_url, so the credential could not be tested."
		report["api"] = "ERROR the host answered but /asset-entitlements is not there: base_url is not the Dell API root. Unset WARRANTY_BASE_URL or point it at the API root."
	case status > 0:
		report["credentials"] = fmt.Sprintf("WARN not verified (HTTP %d from /asset-entitlements): the endpoint did not confirm the credential.", status)
	default:
		report["credentials"] = fmt.Sprintf("ERROR not verified: %s", err)
	}
}

// doctorMintRefused separates a refused credential from a network failure: the
// first request mints, so a wrong Dell secret reaches doctor as a mint error,
// which the generated branch would call "API unreachable".
func doctorMintRefused(err error) bool {
	return err != nil && strings.Contains(err.Error(), "client_credentials mint: HTTP 4")
}

func applyWarrantyDoctor(ctx context.Context, report map[string]any, cfg *config.Config) {
	states := vendorStates(cfg)
	vendors := map[string]any{}
	configured, unset := []string{}, []string{}
	for _, v := range warranty.Vendors {
		st := states[v]
		status := "ok"
		if !st.Configured {
			status = "info"
			unset = append(unset, v)
		} else {
			configured = append(configured, v)
		}
		vendors[v] = map[string]any{"status": status, "configured": st.Configured, "needs": st.Needs, "enrollment": st.Enrollment}
	}
	report["vendors"] = vendors
	switch {
	case len(configured) == 0:
		report["vendors_summary"] = "WARN no vendor credentials set: lookups report vendor_unconfigured until one is (see vendors)"
	case len(unset) == 0:
		report["vendors_summary"] = "OK dell, lenovo and apple configured"
	default:
		report["vendors_summary"] = fmt.Sprintf("OK %s configured; %s optional and not set", strings.Join(configured, ", "), strings.Join(unset, ", "))
	}
	// The generated checks treat the Dell pair as required; here every
	// vendor is optional, so a missing Dell pair is information, not an error.
	if !states[warranty.Dell].Configured {
		report["env_vars"] = "INFO Dell not configured (optional; see vendors)"
	}
	delete(report, "tier_env_vars") // superseded by vendors
	if db, err := openWarrantyRead(ctx); err == nil && db != nil {
		defer db.Close()
		if devices, err := db.ListDevices(ctx, ""); err == nil {
			customers := map[string]bool{}
			never := 0
			for _, d := range devices {
				customers[strings.ToLower(d.Customer)] = true
				if d.LastChecked == "" {
					never++
				}
			}
			report["mirror"] = map[string]any{"devices": len(devices), "customers": len(customers), "never_checked": never}
		}
	}
}

// warrantyAgentContextEnvVars is agent-context's credential list: all three
// vendors, each optional (the generated list names only the Dell pair, as
// required).
func warrantyAgentContextEnvVars() []agentContextAuthEnvVar {
	v := func(name, desc string) agentContextAuthEnvVar {
		return agentContextAuthEnvVar{Name: name, Kind: "per_call", Required: false, Sensitive: true, Description: desc}
	}
	return []agentContextAuthEnvVar{
		v("WARRANTY_DELL_CLIENT_ID", "Optional. Dell TechDirect client ID (Warranty API); also storable with auth login."),
		v("WARRANTY_DELL_CLIENT_SECRET", "Optional. Dell TechDirect client secret; also storable with auth login."),
		v("WARRANTY_LENOVO_CLIENT_KEY", "Optional. Lenovo eSupport ClientID token from your Lenovo account manager. Environment only."),
		v("WARRANTY_APPLE_CLIENT_ID", "Optional. Apple Business API account client ID (BUSINESSAPI.uuid). Environment only."),
		v("WARRANTY_APPLE_KEY_ID", "Optional. Key ID of the Apple Business API account's private key. Environment only."),
		v("WARRANTY_APPLE_PRIVATE_KEY", "Optional. Apple Business ES256 private key: PEM text or a path to the .pem file. Environment only."),
	}
}
