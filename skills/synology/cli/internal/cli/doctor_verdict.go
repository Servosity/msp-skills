// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "doctor-verdict-tracks-reality" entry.

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

// doctorBaseURLEnv names the variable that points the CLI at a NAS, so every
// remedy tells the operator the fix and not only the symptom.
const doctorBaseURLEnv = "SYNOLOGY_BASE_URL"

// doctorSessionProbePath is a cheap authenticated read every DSM account can
// make, so a valid login is proven end to end (login plus a call that carries
// the session).
const doctorSessionProbePath = "/webapi/entry.cgi?api=SYNO.DSM.Info&method=getinfo&version=2"

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
	report["transport"] = doctorTransport(n)
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
	if ok, verdict, credVerdict := doctorProbeDSMAPI(ctx, hc, n.URL); !ok {
		// The Credentials row names the same reason kind as the API row.
		report["api"] = verdict
		setCred(credVerdict)
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
	_, err = c.Get(ctx, doctorSessionProbePath, nil)
	setCred(doctorClassifyLogin(c, err))
}

// doctorTransport reports how the DSM password and session travel: over
// plain http:// they cross the network unencrypted, which every login also
// warns about on stderr (dsm.WarnPlainHTTP), so doctor says WARN.
func doctorTransport(n fleet.NAS) string {
	switch {
	case dsm.IsPlainHTTP(n.URL):
		return "WARN " + dsm.PlainHTTPWarning(n.URL)
	case n.TLSFingerprint != "":
		return "https (certificate pinned by SYNOLOGY_TLS_FINGERPRINT)"
	case n.Insecure:
		return "https (certificate verification off by SYNOLOGY_SKIP_TLS_VERIFY; prefer SYNOLOGY_TLS_FINGERPRINT)"
	}
	return "https (certificate verified)"
}

// doctorProbeDSMAPI reports the host's answer to dsm.ProbeDSM, the same
// unauthenticated SYNO.API.Info check dsm.Login runs before it sends any
// password: doctor and every login share one rule for "this is a DSM". When
// the probe fails it returns the API row and the Credentials row, both named
// after the same reason kind (not DSM, certificate pin mismatch, certificate
// failed verification, unreachable), so the Credentials row never blames
// SYNOLOGY_BASE_URL for a NAS that is down or a certificate that is refused.
func doctorProbeDSMAPI(ctx context.Context, hc *http.Client, origin string) (ok bool, api, credentials string) {
	wrongHost := fmt.Sprintf("point %s at the NAS's DSM address, for example https://<nas>:5001", doctorBaseURLEnv)
	err := dsm.ProbeDSM(ctx, hc, origin)
	var notDSM *dsm.NotDSMError
	switch {
	case err == nil:
		return true, "reachable (DSM WebAPI answered SYNO.API.Info)", ""
	case errors.As(err, &notDSM):
		return false, fmt.Sprintf("ERROR %s - %s is not a DSM WebAPI; %s", notDSM.Reason, doctorBaseURLEnv, wrongHost),
			doctorNotVerified(doctorBaseURLEnv + " does not answer as a DSM WebAPI")
	}
	switch dsm.TransportKind(err) {
	case dsm.KindPinMismatch:
		// Its own verdict: the NAS answered, with a certificate other than the
		// pinned one (usually a renewal), so this is not "unreachable".
		return false, "ERROR certificate pin mismatch: " + err.Error(),
			doctorNotVerified("the NAS certificate does not match the pinned SHA-256 fingerprint (see API)")
	case dsm.KindCertificate:
		return false, "ERROR certificate: " + err.Error(),
			doctorNotVerified("the NAS certificate failed verification (see API)")
	}
	return false, "ERROR unreachable: " + err.Error(),
		doctorNotVerified("the NAS did not answer (" + err.Error() + ")")
}

// doctorNotVerified is the Credentials row when the probe stopped doctor
// before any password was sent.
func doctorNotVerified(reason string) string {
	return "ERROR not verified: " + reason + ", so the password was not sent"
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
