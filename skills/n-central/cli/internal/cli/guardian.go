// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/ncauth"
)

// guardianCheck is one named health check with a status and human detail.
type guardianCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // PASS | WARN | FAIL
	Detail string `json:"detail"`
}

// guardianReport is the machine-readable health report.
type guardianReport struct {
	TokenValid       bool            `json:"tokenValid"`
	PasswordDaysLeft int             `json:"passwordDaysLeft"`
	OkErrorDetected  bool            `json:"okErrorDetected"`
	Checks           []guardianCheck `json:"checks"`
}

func newNovelGuardianCmd(flags *rootFlags) *cobra.Command {
	var flagPasswordSet string
	var flagPolicyDays int

	cmd := &cobra.Command{
		Use:   "guardian",
		Short: "Validate the access token, warn when the API user's password (and thus the JWT) is about to expire, and detect HTTP-200 error bodies.",
		Long: `A CI-wireable health check for N-central API access. Runs three checks:

  1. Token validity — exchanges the JWT and validates the access token.
  2. Password expiry — N-central's API-user password expiry (default 90 days)
     silently invalidates the JWT. Pass --password-set YYYY-MM-DD to track it;
     WARN under 14 days, FAIL when already expired.
  3. 200-OK errors — N-central sometimes returns an error message in a 200 body;
     guardian scans /server-info for one.

Exits non-zero when the token is invalid or the password is already expired,
so it can gate a pipeline.`,
		Example: `  n-central-cli guardian
  n-central-cli guardian --password-set 2026-03-01 --password-policy-days 90 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if flagPolicyDays <= 0 {
				flagPolicyDays = 90
			}

			report := guardianReport{PasswordDaysLeft: -1}

			// Verify-mode: skip every live call and emit a clean stub (exit 0).
			if cliutil.IsVerifyEnv() {
				report.TokenValid = true
				report.Checks = append(report.Checks, guardianCheck{
					Name: "token", Status: "PASS", Detail: "skipped under verify mode",
				})
				return guardianOutput(cmd, flags, report, nil)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// (a) Token validity.
			var fatal error
			if verr := ncauth.Validate(cmd.Context(), c.Config); verr != nil {
				report.TokenValid = false
				report.Checks = append(report.Checks, guardianCheck{
					Name: "token", Status: "FAIL", Detail: fmt.Sprintf("access token invalid: %v", verr),
				})
				fatal = fmt.Errorf("guardian: access token is invalid")
			} else {
				report.TokenValid = true
				report.Checks = append(report.Checks, guardianCheck{
					Name: "token", Status: "PASS", Detail: "access token validated",
				})
			}

			// (b) Password expiry.
			if flagPasswordSet != "" {
				set, ok := parseDateFlag(flagPasswordSet)
				if !ok {
					return usageErr(fmt.Errorf("invalid --password-set %q: expected YYYY-MM-DD", flagPasswordSet))
				}
				expiry := set.AddDate(0, 0, flagPolicyDays)
				daysLeft := int(time.Until(expiry).Hours() / 24)
				report.PasswordDaysLeft = daysLeft
				switch {
				case daysLeft < 0:
					report.Checks = append(report.Checks, guardianCheck{
						Name: "password", Status: "FAIL",
						Detail: fmt.Sprintf("API-user password expired %d day(s) ago (set %s, %d-day policy); N-central silently invalidates the JWT on password expiry", -daysLeft, flagPasswordSet, flagPolicyDays),
					})
					if fatal == nil {
						fatal = fmt.Errorf("guardian: API-user password has expired")
					}
				case daysLeft < 14:
					report.Checks = append(report.Checks, guardianCheck{
						Name: "password", Status: "WARN",
						Detail: fmt.Sprintf("API-user password expires in %d day(s); rotate it before N-central invalidates the JWT", daysLeft),
					})
				default:
					report.Checks = append(report.Checks, guardianCheck{
						Name: "password", Status: "PASS",
						Detail: fmt.Sprintf("API-user password has %d day(s) left", daysLeft),
					})
				}
			} else {
				report.Checks = append(report.Checks, guardianCheck{
					Name: "password", Status: "WARN",
					Detail: "password expiry not checked; pass --password-set YYYY-MM-DD. N-central's default 90-day API-user password expiry silently invalidates the JWT — the #1 documented N-central API outage.",
				})
			}

			// (c) 200-OK error detection.
			raw, gerr := c.Get(cmd.Context(), "/server-info", nil)
			if gerr != nil {
				report.Checks = append(report.Checks, guardianCheck{
					Name: "ok-error", Status: "WARN",
					Detail: fmt.Sprintf("could not probe /server-info: %v", classifyAPIError(gerr, flags)),
				})
			} else if msg := guardianScanOKError(raw); msg != "" {
				report.OkErrorDetected = true
				report.Checks = append(report.Checks, guardianCheck{
					Name: "ok-error", Status: "FAIL",
					Detail: fmt.Sprintf("HTTP 200 response carried an error message: %s", msg),
				})
			} else {
				report.Checks = append(report.Checks, guardianCheck{
					Name: "ok-error", Status: "PASS",
					Detail: "/server-info returned a clean 200",
				})
			}

			return guardianOutput(cmd, flags, report, fatal)
		},
	}
	cmd.Flags().StringVar(&flagPasswordSet, "password-set", "", "Date the API user's password was last set (YYYY-MM-DD)")
	cmd.Flags().IntVar(&flagPolicyDays, "password-policy-days", 90, "Password-expiry policy length in days")
	return cmd
}

// guardianOutput renders the report (human or JSON) then returns fatal so the
// process exits non-zero when a hard check failed. The report is always shown
// first so a failing CI run still surfaces the detail.
func guardianOutput(cmd *cobra.Command, flags *rootFlags, report guardianReport, fatal error) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		for _, ck := range report.Checks {
			marker := ck.Status
			switch ck.Status {
			case "PASS":
				marker = green("PASS")
			case "WARN":
				marker = yellow("WARN")
			case "FAIL":
				marker = red("FAIL")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", marker, ck.Name, ck.Detail)
		}
	} else if err := flags.printJSON(cmd, report); err != nil {
		return err
	}
	return fatal
}

// guardianScanOKError scans an HTTP-200 JSON body for an error-message field.
// N-central occasionally returns an error in the body of a 200 response;
// detecting it prevents a green pipeline masking a real failure. Returns the
// error text when found, else "".
func guardianScanOKError(raw json.RawMessage) string {
	obj := decodeObj(raw)
	if obj == nil {
		return ""
	}
	// Direct error-message fields.
	for _, key := range []string{"error", "errorMessage", "Error Message", "errorMsg"} {
		if v := firstField(obj, key); v != nil {
			if s := asString(v); s != "" && s != "false" && s != "0" {
				return s
			}
		}
	}
	// A "message" field is only treated as an error when a sibling status/
	// success field indicates failure — a bare "message" is often benign.
	if v := firstField(obj, "message"); v != nil {
		if s := asString(v); s != "" {
			if guardianLooksLikeFailure(obj) {
				return s
			}
		}
	}
	return ""
}

// guardianLooksLikeFailure inspects status/success-shaped fields to decide
// whether a 200 body actually represents a failure.
func guardianLooksLikeFailure(obj map[string]any) bool {
	if v := firstField(obj, "success"); v != nil {
		if asString(v) == "false" {
			return true
		}
	}
	if v := firstField(obj, "status", "result"); v != nil {
		switch asString(v) {
		case "error", "fail", "failure", "failed":
			return true
		}
	}
	if v := firstField(obj, "errorCode", "error_code"); v != nil {
		if n, ok := asInt(v); ok && n != 0 {
			return true
		}
	}
	return false
}
