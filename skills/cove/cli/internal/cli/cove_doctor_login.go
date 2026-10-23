// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cove-pp-cli/internal/coverpc"
)

// Hand-owned (handfixes.json: doctor-verifies-cove-login). Cove authenticates
// with a JSON-RPC Login (COVE_PARTNER, COVE_USERNAME, COVE_PASSWORD), not an
// HTTP header, so the generated doctor reported "auth: not required" and
// "no credential is configured" with all three set, and never tried the login
// every real command depends on.

// coveDoctorAuth is the auth row: the scheme Cove actually uses.
const coveDoctorAuth = "JSON-RPC Login (COVE_PARTNER, COVE_USERNAME, COVE_PASSWORD)"

// coveDoctorLogin performs one Login and reports the verdict. It never caches
// the session (a probe leaves no state behind) and never prints the server's
// response text: a failure is reported by Cove error number only, so nothing
// the server reflects back (a credential included) reaches the report.
func coveDoctorLogin(ctx context.Context, baseURL string, timeout time.Duration) string {
	c := coverpc.New(baseURL)
	c.SessionPath = ""
	if timeout > 0 {
		c.HTTP.Timeout = timeout
	}
	if _, err := c.Login(ctx); err != nil {
		var rpcErr *coverpc.RPCError
		if errors.As(err, &rpcErr) {
			return fmt.Sprintf("ERROR Login refused (Cove error %d); check COVE_PARTNER (the full customer string from the console dropdown), COVE_USERNAME and COVE_PASSWORD, then run `cove-cli auth login`", rpcErr.Data)
		}
		return "ERROR Login failed: no Cove JSON-RPC session came back; check COVE_BASE_URL and run `cove-cli auth login` for the full error"
	}
	return "ok (JSON-RPC Login accepted)"
}
