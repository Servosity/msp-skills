// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "region-normalized-and-validated".
//
// ESET_PROTECT_REGION is substituted into every ESET Connect host. The token
// host was minted from the lowercased value while service URLs kept the case
// the operator typed, so ESET_PROTECT_REGION=US minted a token and then had
// every API call refused by the credential host pin with a misleading
// "not an ESET Connect host" security error. NormalizeRegion is the one place
// the value is cleaned (config.Load, sync --path-context region=), and
// ValidateRegion refuses anything that is not an ESET Connect region with a
// message naming the variable and the valid values.
package config

import (
	"fmt"
	"strings"
)

// Where Config.TemplateVars["region"] came from (Config.RegionSource).
// Round 3: a config-file region used to be overwritten with eu whenever
// ESET_PROTECT_REGION was unset, so a token saved for us was sent to eu.
const (
	RegionSourceEnv  = "ESET_PROTECT_REGION"
	RegionSourceFile = "the config file's [template_vars] region"
)

// RegionSourceName names the source of cfg's region for messages: the env
// variable, the config file (with its path), or "the default".
func RegionSourceName(cfg *Config) string {
	switch {
	case cfg == nil || cfg.RegionSource == "":
		return "the default"
	case cfg.RegionSource == RegionSourceFile && cfg.Path != "":
		return RegionSourceFile + " (" + cfg.Path + ")"
	}
	return cfg.RegionSource
}

// EsetRegions are the ESET Connect regions, in the order ESET documents them.
var EsetRegions = []string{"eu", "de", "us", "ca", "jpn"}

// NormalizeRegion trims, strips a pasted scheme or trailing slash and
// lowercases a region value.
func NormalizeRegion(v string) string {
	return strings.ToLower(normalizeEndpointTemplateValue(strings.TrimSpace(v)))
}

// ValidateRegion returns nil for an ESET Connect region (already normalized)
// and otherwise an error naming source (ESET_PROTECT_REGION, or the flag that
// set it) and the valid values.
func ValidateRegion(region, source string) error {
	for _, r := range EsetRegions {
		if region == r {
			return nil
		}
	}
	return fmt.Errorf("invalid %s %q: ESET Connect regions are %s (set ESET_PROTECT_REGION to the region of your ESET PROTECT tenant)", source, region, strings.Join(EsetRegions, ", "))
}
