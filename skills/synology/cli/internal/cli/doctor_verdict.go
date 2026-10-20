// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "doctor-verdict-tracks-reality" entry.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
)

// doctorBaseURLEnv names the variable that points the CLI at a NAS, so every
// remedy tells the operator the fix and not only the symptom.
const doctorBaseURLEnv = "SYNOLOGY_BASE_URL"

// doctorAPIInfoPath is DSM's unauthenticated API discovery call. Every DSM
// answers it with {"success":true,"data":{...}}; a web page, a proxy or any
// other server does not.
const doctorAPIInfoPath = "/webapi/query.cgi?api=SYNO.API.Info&method=query&version=1&query=SYNO.API.Auth"

// doctorCredentialProbePath is a cheap authenticated read every DSM account
// can make, so a valid login is proven end to end (login plus a call that
// carries the session).
const doctorCredentialProbePath = "/webapi/entry.cgi?api=SYNO.DSM.Info&method=getinfo&version=2"

// doctorDSMVerdict replaces the generated reachability and credential checks.
//
// The generated doctor called any HTTP response to GET / "reachable" (DSM's
// login page, a proxy, a router: "reachable (HTML body at /)") and only ever
// printed "present, not verified" for a credential; with SYNOLOGY_ACCOUNT and
// SYNOLOGY_PASSWORD set it printed no credentials row at all and reported
// auth "not configured", because the generated auth check looks only at an
// auth_header. Here: refuse the placeholder address and name SYNOLOGY_BASE_URL;
// prove the host is a DSM WebAPI with SYNO.API.Info before any password is
// sent; then log in and make one authenticated read, classifying the DSM
// answer (400 rejected, 403 two-factor code needed, 105 scope-limited, ...).
func doctorDSMVerdict(ctx context.Context, flags *rootFlags, cfg *config.Config, report map[string]any) {
	setCred := func(v string) {
		if _, set := report["credentials"]; !set {
			report["credentials"] = v
		}
	}
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || doctorBaseURLIsPlaceholder(cfg.BaseURL) || dsm.PlaceholderOrigin(cfg.BaseURL) {
		report["api"] = fmt.Sprintf("ERROR not configured: %s is unset (the shipped https://nas.example.com:5001 is a placeholder); set it to your NAS, for example https://<nas>:5001", doctorBaseURLEnv)
		setCred(fmt.Sprintf("skipped (no NAS address: set %s)", doctorBaseURLEnv))
		return
	}
	n, ok, err := defaultNAS(flags)
	if err != nil || !ok {
		msg := "the address is not usable"
		if err != nil {
			msg = err.Error()
		}
		report["api"] = fmt.Sprintf("ERROR %s: %s", doctorBaseURLEnv, msg)
		setCred("skipped (the NAS address was refused)")
		return
	}
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	hc, err := n.HTTPClient(timeout)
	if err != nil {
		report["api"] = "ERROR " + err.Error()
		setCred("skipped (the NAS address was refused)")
		return
	}
	if ok, verdict := doctorProbeDSMAPI(ctx, hc, n.URL); !ok {
		report["api"] = verdict
		setCred("ERROR not verified: " + doctorBaseURLEnv + " does not answer as a DSM WebAPI, so the password was not sent")
		return
	} else {
		report["api"] = verdict
	}
	configured, missing := dsmCredentialState(cfg)
	switch {
	case !configured:
		setCred(fmt.Sprintf("ERROR not configured: set %s (synology-cli auth setup prints the macOS/Linux and PowerShell lines)", strings.Join(missing, " and ")))
		return
	case cfg.SynologyAccount == "" || cfg.SynologyPassword == "":
		setCred("WARN not verified: only a raw session id (auth_header) is configured; set SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD so doctor can verify a login")
		return
	}
	c, err := dsmClient(flags, n)
	if err != nil {
		setCred("ERROR not verified: " + err.Error())
		return
	}
	defer func() { _ = c.Logout(context.WithoutCancel(ctx)) }()
	_, err = c.Get(ctx, doctorCredentialProbePath, nil)
	setCred(doctorClassifyLogin(c, err))
}

// doctorProbeDSMAPI asks the host for SYNO.API.Info without credentials and
// reports whether it answered as DSM.
func doctorProbeDSMAPI(ctx context.Context, hc *http.Client, origin string) (bool, string) {
	wrongHost := fmt.Sprintf("point %s at the NAS's DSM address, for example https://<nas>:5001", doctorBaseURLEnv)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(origin, "/")+doctorAPIInfoPath, nil)
	if err != nil {
		return false, "ERROR " + err.Error()
	}
	resp, err := hc.Do(req)
	if err != nil {
		return false, "ERROR unreachable: " + dsm.DescribeTransport(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, "ERROR unreachable: the response could not be read"
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("ERROR the host answered HTTP %d for SYNO.API.Info, which DSM answers with HTTP 200 - %s is not a DSM WebAPI; %s", resp.StatusCode, doctorBaseURLEnv, wrongHost)
	}
	if doctorBodyLooksLikeHTML(body) {
		return false, "ERROR the host answered SYNO.API.Info with an HTML page, not DSM's WebAPI; " + wrongHost
	}
	data, code, failed := dsm.Envelope(body)
	switch {
	case failed:
		return false, fmt.Sprintf("ERROR DSM refused SYNO.API.Info (%s); %s", dsm.ErrorText(code, "SYNO.API.Info"), wrongHost)
	case data == nil:
		return false, "ERROR the host answered SYNO.API.Info without DSM's {\"success\":true,\"data\":...} envelope - not a DSM WebAPI; " + wrongHost
	}
	var apis map[string]json.RawMessage
	if json.Unmarshal(data, &apis) != nil {
		return false, "ERROR the host answered SYNO.API.Info with an unexpected payload - not a DSM WebAPI; " + wrongHost
	}
	return true, "reachable (DSM WebAPI answered SYNO.API.Info)"
}

// doctorClassifyLogin turns the login-plus-read outcome into a verdict. Every
// message passes through c.Redact, so nothing the NAS echoes can print a
// password, one-time code, device token or session id.
func doctorClassifyLogin(c *dsm.Client, err error) string {
	if err == nil {
		return "valid (DSM login and an authenticated SYNO.DSM.Info read succeeded)"
	}
	var de *dsm.Error
	if errors.As(err, &de) {
		text := c.Redact(de.Error())
		switch {
		case de.IsAuth() && (de.Code == 403):
			return "WARN not verified: " + text
		case de.IsAuth():
			return "ERROR rejected: " + text
		case de.Code == 105 || de.Code == 160:
			return "scope-limited: logged in, but " + text
		case dsm.SessionExpired(de.Code):
			return "ERROR rejected: DSM dropped the new session (" + text + ")"
		}
		return "ERROR not verified: " + text
	}
	return "ERROR not verified: " + c.Redact(err.Error())
}
