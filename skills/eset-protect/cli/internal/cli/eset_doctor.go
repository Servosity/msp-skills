// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: makes `doctor` report a verdict that tracks reality for the
// ESET Connect password grant (handfixes.json: doctor-verdict-tracks-reality).
//
// The generated doctor only saw a credential once an Authorization header was
// already stamped. ESET mints its bearer from ESET_PROTECT_USERNAME and
// ESET_PROTECT_PASSWORD inside the client hook, so doctor printed "Auth: not
// configured" for a configured API user, turned a rejected mint into "API:
// client init error", and never checked the token at all ("present, not
// verified" at best). Doctor now names the password grant on the Auth row,
// reports a rejected mint as an Auth and Credentials failure while still
// probing reachability, and verifies the token with one real authenticated
// read (GET device-management /v1/device_groups?pageSize=1, the same probe
// `auth login` uses).
//
// The API row is held to the same standard. A base URL that answers that
// probe with HTTP 404 or an HTML page is not the ESET Connect API, whatever
// "/" said, so the row is an ERROR naming ESET_PROTECT_BASE_URL. With neither
// ESET_PROTECT_REGION nor a base URL override set, the default eu region is
// a guess, so the row is a WARN that says so and names both variables.

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/config"
	"eset-protect-pp-cli/internal/secrets"
)

// esetDoctorAuthState is the Auth row's presence check, aware of the
// password grant.
func esetDoctorAuthState(cfg *config.Config) (bool, string) {
	if cfg != nil && cfg.EsetProtectToken == "" && esetHasCredentials(cfg) {
		return true, "password grant (ESET_PROTECT_USERNAME / ESET_PROTECT_PASSWORD)"
	}
	return doctorAuthConfiguredState(cfg)
}

// esetDoctorClient returns doctor's client. When the token mint is refused it
// records that on the Auth and Credentials rows and returns a client that
// carries no credential, so the API row still reports reachability.
func esetDoctorClient(flags *rootFlags, cfg *config.Config, report map[string]any) (*client.Client, error) {
	c, err := flags.newClient()
	if err == nil {
		return c, nil
	}
	var mint *esetTokenError
	if cfg == nil || cfg.EsetProtectToken != "" || !esetHasCredentials(cfg) || !errors.As(err, &mint) {
		return nil, err
	}
	report["auth"] = "ERROR " + err.Error()
	report["credentials"] = "ERROR rejected: no ESET Connect token could be minted from ESET_PROTECT_USERNAME / ESET_PROTECT_PASSWORD"
	if !mint.rejected {
		// handfixes.json: mint-exit-code-tracks-rejection. The token service
		// did not answer with a token; that says nothing about the password.
		report["credentials"] = "ERROR not verified: the ESET Connect token request failed without rejecting ESET_PROTECT_USERNAME / ESET_PROTECT_PASSWORD (network, proxy, rate limit or an ESET outage); retry"
	}
	anon := *cfg
	anon.EsetProtectUsername, anon.EsetProtectPassword, anon.EsetProtectToken = "", "", ""
	anon.AccessToken, anon.RefreshToken, anon.AuthHeaderVal = "", "", ""
	c = client.New(&anon, flags.timeout, flags.rateLimit)
	c.HTTPClient.Transport = &client.EsetTransport{Base: c.HTTPClient.Transport, AllowHost: esetHostAllower(&anon)}
	return c, nil
}

// esetDoctorProbeCredentials verifies the bearer with one authenticated read
// and classifies the answer.
func esetDoctorProbeCredentials(ctx context.Context, c *client.Client, report map[string]any) {
	region, err := esetRegion(c.Config)
	if err != nil {
		report["credentials"] = "ERROR " + err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.NoCache = true
	_, err = c.Get(ctx, "https://"+region+".device-management.eset.systems/v1/device_groups", map[string]string{"pageSize": "1"})
	var apiErr *client.APIError
	switch {
	case err == nil:
		report["credentials"] = "valid: device-management accepted the token (GET /v1/device_groups)"
	case errors.As(err, &apiErr) && apiErr.StatusCode == 401:
		report["credentials"] = "ERROR rejected: device-management answered HTTP 401 to the token"
	case errors.As(err, &apiErr) && apiErr.StatusCode == 403:
		report["credentials"] = "WARN scope-limited: device-management answered HTTP 403; give the API user ESET Connect access to device management"
	case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
		esetDoctorWrongBase(c, region, report, "HTTP 404")
	case err != nil && strings.Contains(err.Error(), "returned HTML instead of JSON"):
		esetDoctorWrongBase(c, region, report, "an HTML page")
	default:
		report["credentials"] = "ERROR probe failed: " + err.Error()
	}
}

// esetDoctorWrongBase marks the API and Credentials rows when the base URL
// answered the device-management probe with something the ESET Connect API
// never sends for it: the base points at another service or a web UI.
func esetDoctorWrongBase(c *client.Client, region string, report map[string]any, answer string) {
	base := strings.TrimRight(client.ResolvedBaseURL(c.Config.BaseURL, c.Config.TemplateVars), "/")
	report["api"] = fmt.Sprintf("ERROR wrong base URL: %s answered GET /v1/device_groups with %s, so it is not the ESET Connect device-management API; unset ESET_PROTECT_BASE_URL (and base_url in the config file) or point it at https://%s.device-management.eset.systems", base, answer, region)
	report["credentials"] = fmt.Sprintf("ERROR not verified: GET /v1/device_groups answered with %s at %s, which is not the ESET Connect API", answer, base)
}

// esetDoctorRegionDefaulted is true when nothing chose the ESET region: no
// ESET_PROTECT_REGION and a base URL that is still the {region} template (no
// ESET_PROTECT_BASE_URL or config-file base_url override), so requests went
// to the default eu data center.
func esetDoctorRegionDefaulted(cfg *config.Config) bool {
	return cfg != nil && strings.TrimSpace(cliutil.EnvOverride("ESET_PROTECT_REGION")) == "" && cfg.RegionSource == "" && strings.Contains(cfg.BaseURL, "{region}")
}

// esetDoctorRegionFromFile is true when the region came from the config
// file, not ESET_PROTECT_REGION and not the default.
func esetDoctorRegionFromFile(cfg *config.Config) bool {
	return cfg != nil && cfg.RegionSource == config.RegionSourceFile && strings.Contains(cfg.BaseURL, "{region}")
}

// esetDoctorRegionVerdict turns the API row into a WARN when the region was
// defaulted. eu is a legitimate default, so the row never claims the URL is a
// placeholder; it says what was assumed and how to choose. A worse verdict
// (unreachable, wrong base) keeps its severity and gains the same remedy.
func esetDoctorRegionVerdict(cfg *config.Config, report map[string]any) {
	defaulted, fromFile := esetDoctorRegionDefaulted(cfg), esetDoctorRegionFromFile(cfg)
	if !defaulted && !fromFile {
		// A region set by ESET_PROTECT_REGION is a choice, not a note: the
		// row keeps its verdict and only names the region and its source
		// (handfixes.json: region-never-persisted).
		if api, _ := report["api"].(string); api != "" && cfg != nil && cfg.RegionSource == config.RegionSourceEnv && strings.Contains(cfg.BaseURL, "{region}") {
			if region, err := esetRegion(cfg); err == nil {
				report["api"] = fmt.Sprintf("%s (region %s from %s)", api, region, config.RegionSourceName(cfg))
			}
		}
		return
	}
	region, err := esetRegion(cfg)
	if err != nil {
		return
	}
	note := fmt.Sprintf("ESET_PROTECT_REGION is not set, so the default %s region was used; set ESET_PROTECT_REGION to eu, de, us, ca or jpn to match the tenant, or ESET_PROTECT_BASE_URL for a custom endpoint", region)
	if fromFile {
		// The region INFO row names where the region came from.
		note = fmt.Sprintf("ESET_PROTECT_REGION is not set, so the %s region from %s was used; ESET_PROTECT_REGION overrides it", region, config.RegionSourceName(cfg))
	}
	api, _ := report["api"].(string)
	// The documented eu default is a valid configuration, so the note is an
	// INFO row (doctor --fail-on warn passes on it); a failing or warning API
	// verdict keeps its own severity with the note appended (handfixes.json:
	// doctor-fail-on-matches-rows).
	switch {
	case api == "":
		report["api"] = "INFO " + note
	case doctorRowVerdict(api) == doctorVerdictFail || doctorRowVerdict(api) == doctorVerdictWarn:
		report["api"] = api + "; " + note
	default:
		report["api"] = fmt.Sprintf("INFO %s (https://%s.device-management.eset.systems: %s)", note, region, api)
	}
}

// esetMaskDoctorReport runs every string in the doctor report (rows, hints,
// nested cache and paths sections) through the secret registry before it is
// printed or gated on (handfixes.json: secret-registry-masks-every-boundary).
// A row's verdict survives the masking: a row withheld whole keeps the
// verdict word it had, so --fail-on and the printed indicator never turn a
// failure into OK.
func esetMaskDoctorReport(report map[string]any) {
	for k, v := range report {
		report[k] = esetMaskDoctorValue(v)
	}
}

func esetMaskDoctorValue(v any) any {
	switch t := v.(type) {
	case string:
		m := secrets.Mask(t)
		if m == t {
			return t
		}
		if before, after := doctorRowVerdict(t), doctorRowVerdict(m); after != before {
			switch before {
			case doctorVerdictFail:
				m = "ERROR " + m
			case doctorVerdictWarn:
				m = "WARN " + m
			case doctorVerdictInfo:
				m = "INFO " + m
			}
		}
		return m
	case map[string]any:
		for k, x := range t {
			t[k] = esetMaskDoctorValue(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = esetMaskDoctorValue(x)
		}
		return t
	case []string:
		for i, x := range t {
			t[i] = secrets.Mask(x)
		}
		return t
	case []map[string]string:
		for _, m := range t {
			for k, x := range m {
				m[k] = secrets.Mask(x)
			}
		}
		return t
	case map[string]string:
		for k, x := range t {
			t[k] = secrets.Mask(x)
		}
		return t
	case error:
		if m := secrets.Mask(t.Error()); m != t.Error() {
			return m
		}
		return t
	}
	return v
}
