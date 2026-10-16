// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
)

// postureRow is one firewall's insurance posture. Every *_status field is
// pass, warn, fail or unknown; unknown means the data could not be read.
type postureRow struct {
	Serial          string   `json:"serial"`
	Name            string   `json:"name"`
	Tenant          string   `json:"tenant_id,omitempty"`
	Online          *bool    `json:"online"` // null when NSM did not report it
	Overall         string   `json:"overall"`
	Firmware        string   `json:"firmware"`
	LatestFirmware  string   `json:"latest_firmware,omitempty"`
	VersionsBehind  *int     `json:"versions_behind,omitempty"`
	FirmwareStatus  string   `json:"firmware_status"`
	Advisories      []string `json:"advisories"`
	AdvisoryStatus  string   `json:"advisory_status"`
	Model           string   `json:"model,omitempty"`
	EndOfSupport    string   `json:"end_of_support,omitempty"`
	SupportStatus   string   `json:"support_status"`
	SoonestLicence  string   `json:"soonest_licence,omitempty"`
	LicenceDaysLeft *int     `json:"licence_days_left,omitempty"`
	LicenceStatus   string   `json:"licence_status"`
	AdminTOTP       string   `json:"admin_totp"`
	SSLVPNZones     []string `json:"sslvpn_zones"`
	SSLVPNWAN       string   `json:"sslvpn_wan_status"`
	SSLVPNAuth      string   `json:"sslvpn_auth,omitempty"`
	UsersWithoutMFA *int     `json:"sslvpn_users_without_mfa,omitempty"`
	UsersMFAUnknown *int     `json:"sslvpn_users_mfa_unknown,omitempty"`
	UserMFA         string   `json:"user_mfa_status"`
	Errors          []string `json:"errors,omitempty"`
}

// checks is the per-check status map stored in posture snapshots.
func (r postureRow) checks() map[string]string {
	online := posture.Unknown
	if r.Online != nil {
		online = posture.Pass
		if !*r.Online {
			online = posture.Fail
		}
	}
	return map[string]string{
		"online":         online,
		"firmware":       r.FirmwareStatus,
		"advisories":     r.AdvisoryStatus,
		"end_of_support": r.SupportStatus,
		"licences":       r.LicenceStatus,
		"admin_totp":     r.AdminTOTP,
		"sslvpn_wan":     r.SSLVPNWAN,
		"user_mfa":       r.UserMFA,
	}
}

type postureInputs struct {
	vulnerable    map[string][]string // serial -> advisory IDs; nil when the fetch failed
	vulnErr       error
	mswKey        bool
	licenceWarn   int
	supportWindow time.Duration
	now           time.Time
}

// evaluateDevice runs the per-firewall reads and grades them. Fetch or parse
// failures become unknown statuses plus an errors entry, never a pass.
func evaluateDevice(ctx context.Context, c *client.Client, d fleetDevice, in postureInputs) postureRow {
	r := postureRow{Serial: d.SerialNumber, Name: d.FriendlyName, Tenant: d.TenantID, Online: d.LiveStatus,
		Firmware: d.FirmwareVersion, Advisories: make([]string, 0), SSLVPNZones: make([]string, 0)}
	fail := func(what string, err error) { r.Errors = append(r.Errors, what+": "+errText(err)) }

	// Firmware currency.
	r.FirmwareStatus = posture.Unknown
	if raw, err := fetchAvailableVersions(ctx, c, d.SerialNumber); err != nil {
		fail("available firmware", err)
	} else if versions, ok := posture.AvailableVersions(raw); !ok {
		r.Errors = append(r.Errors, "available firmware: unexpected response shape")
	} else if behind, newest, ok := posture.FirmwareGap(d.FirmwareVersion, versions); !ok {
		r.Errors = append(r.Errors, "available firmware: running version not reported")
	} else {
		r.VersionsBehind, r.LatestFirmware = &behind, newest
		r.FirmwareStatus = posture.Pass
		if behind > 0 {
			r.FirmwareStatus = posture.Warn
		}
	}

	// Published advisories (one fleet-wide call, joined here).
	switch {
	case in.vulnErr != nil:
		r.AdvisoryStatus = posture.Unknown
		fail("advisories", in.vulnErr)
	case len(in.vulnerable[strings.ToUpper(d.SerialNumber)]) > 0:
		r.Advisories = in.vulnerable[strings.ToUpper(d.SerialNumber)]
		r.AdvisoryStatus = posture.Fail
	case sonicOSVersionPattern.FindString(strings.TrimSpace(d.FirmwareVersion)) == "":
		// Absent from the vulnerable list proves nothing when NSM reported no
		// running SonicOS version to judge.
		r.AdvisoryStatus = posture.Unknown
		r.Errors = append(r.Errors, "advisories: running firmware version not reported")
	default:
		r.AdvisoryStatus = posture.Pass
	}

	// End of support.
	if lc, err := fetchLifecycle(ctx, c, d.SerialNumber); err != nil {
		r.SupportStatus = posture.Unknown
		fail("product life cycle", err)
	} else {
		r.Model, r.EndOfSupport = lc.Model, lc.EOS
		r.SupportStatus = posture.SupportStatus(lc.EOS, in.now, in.supportWindow)
	}

	// Licences (MySonicWall).
	r.LicenceStatus = posture.Unknown
	if !in.mswKey {
		r.Errors = append(r.Errors, "licences: SONICWALL_MSW_API_KEY not set")
	} else if raw, err := mswServiceInfo(ctx, c, d.SerialNumber); err != nil {
		fail("licences", err)
	} else if ls, ok := posture.Licences(raw, in.now); !ok {
		r.Errors = append(r.Errors, "licences: no readable serviceTypeList in the MySonicWall response")
	} else if soon, found := posture.SoonestExpiry(ls); found {
		r.SoonestLicence = soon.Service
		// An expired service may carry no date: its day count stays null.
		r.LicenceDaysLeft = soon.DaysToExpiry
		days := 0
		if soon.DaysToExpiry != nil {
			days = *soon.DaysToExpiry
		}
		switch {
		case soon.Expired || days < 0:
			r.LicenceStatus = posture.Fail
		case posture.AnyUndated(ls):
			// A service with no readable expiry could lapse sooner than the soonest dated one.
			r.LicenceStatus = posture.Unknown
			r.Errors = append(r.Errors, "licences: a service has no readable expiry date")
		case days <= in.licenceWarn:
			r.LicenceStatus = posture.Warn
		default:
			r.LicenceStatus = posture.Pass
		}
	} else if len(ls) == 0 {
		r.Errors = append(r.Errors, "licences: MySonicWall reported no services for this serial")
	} else if posture.AnyUndated(ls) {
		r.Errors = append(r.Errors, "licences: no service has a readable expiry date")
	}

	// SonicOS config through NSM: admin TOTP, SSL-VPN zones and auth, local-user OTP.
	r.AdminTOTP = posture.Unknown
	if raw, err := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/administration/global"); err != nil {
		fail("admin settings", err)
	} else if on, ok := posture.AdminTOTP(raw); !ok {
		r.Errors = append(r.Errors, "admin settings: unexpected response shape")
	} else {
		r.AdminTOTP = posture.Fail
		if on {
			r.AdminTOTP = posture.Pass
		}
	}

	r.SSLVPNWAN, r.UserMFA = posture.Unknown, posture.Unknown
	zonesRaw, err := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/ssl-vpn/server/accesses")
	if err != nil {
		fail("ssl-vpn zones", err)
		return finishRow(r)
	}
	zones, wan, ok := posture.SSLVPNZones(zonesRaw)
	if !ok {
		r.Errors = append(r.Errors, "ssl-vpn zones: unexpected response shape")
		return finishRow(r)
	}
	r.SSLVPNZones = zones
	r.SSLVPNWAN = posture.Pass
	if wan {
		r.SSLVPNWAN = posture.Warn
	}
	if len(zones) == 0 {
		r.UserMFA = posture.Pass // SSL VPN is not enabled on any zone
		return finishRow(r)
	}
	// Without the server settings we cannot tell SAML sign-in from password sign-in.
	serverRaw, err := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/ssl-vpn/server/base")
	if err != nil {
		fail("ssl-vpn server", err)
		return finishRow(r)
	}
	authType, saml, ok := posture.SSLVPNAuth(serverRaw)
	if !ok {
		r.Errors = append(r.Errors, "ssl-vpn server: auth_type not reported or not a documented value")
		return finishRow(r)
	}
	r.SSLVPNAuth = authType
	if saml {
		r.UserMFA = posture.Warn // MFA is enforced (or not) by the SAML provider
		return finishRow(r)
	}
	// Which directories users sign in through: local accounts, RADIUS, LDAP or TACACS+.
	methodsRaw, merr := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/user/authentication/methods")
	if merr != nil {
		fail("user authentication methods", merr)
		return finishRow(r)
	}
	methods, ok := posture.UserAuthMethods(methodsRaw)
	if !ok {
		r.Errors = append(r.Errors, "user authentication methods: auth_method not reported or not a documented value")
		return finishRow(r)
	}
	external, localSignIn := posture.SignInSources(methods)
	if len(external) > 0 {
		r.SSLVPNAuth += "+" + strings.Join(external, "+")
	}
	if !localSignIn {
		r.UserMFA = posture.Warn // MFA is enforced (or not) by the RADIUS/LDAP/TACACS+ directory
		return finishRow(r)
	}
	usersRaw, uerr := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/user/local/users")
	if uerr != nil {
		fail("local users", uerr)
		return finishRow(r)
	}
	groupsRaw, gerr := sonicOSGet(ctx, c, d.SerialNumber, "/firewall/user/local/groups")
	if gerr != nil {
		fail("local groups", gerr) // users without their own OTP become unknown below
		groupsRaw = nil
	}
	accounts, ok := posture.LocalAccounts(usersRaw, groupsRaw)
	if !ok {
		r.Errors = append(r.Errors, "local users: unexpected response shape")
		return finishRow(r)
	}
	missing, undecided := countSSLVPNMFA(accounts)
	// The count is definite only when every SSL-VPN account was read and
	// decided: directory users are never read, and undecided local accounts
	// are counted separately.
	if undecided > 0 {
		r.UsersMFAUnknown = &undecided
		r.Errors = append(r.Errors, fmt.Sprintf("local users: MFA coverage of %d account(s) not reported", undecided))
	} else if len(external) == 0 {
		r.UsersWithoutMFA = &missing
	}
	switch {
	case missing > 0:
		r.UserMFA = posture.Fail
	case undecided > 0:
		r.UserMFA = posture.Unknown
	case len(external) > 0:
		r.UserMFA = posture.Warn // local accounts pass; directory users' MFA is enforced upstream
	default:
		r.UserMFA = posture.Pass
	}
	return finishRow(r)
}

// countSSLVPNMFA counts SSL-VPN accounts with no one-time password (missing)
// and accounts whose OTP or SSL-VPN membership was not reported (undecided).
func countSSLVPNMFA(accounts []posture.Account) (missing, undecided int) {
	for _, a := range accounts {
		switch {
		case a.SSLVPNMember != nil && !*a.SSLVPNMember:
			// confirmed not an SSL-VPN user: cannot change the answer
		case a.Method == "unknown":
			undecided++
		case a.Method != "none":
		case a.SSLVPNMember == nil:
			undecided++ // no OTP, and SSL-VPN membership was not reported
		default:
			missing++
		}
	}
	return missing, undecided
}

// finishRow grades the row. Any recorded read error keeps the overall result at
// unknown or worse, so a partially read firewall can never be an overall pass.
func finishRow(r postureRow) postureRow {
	statuses := []string{r.FirmwareStatus, r.AdvisoryStatus, r.SupportStatus, r.LicenceStatus, r.AdminTOTP, r.SSLVPNWAN, r.UserMFA}
	switch {
	case r.Online == nil:
		statuses = append(statuses, posture.Unknown)
	case !*r.Online:
		statuses = append(statuses, posture.Fail)
	}
	if len(r.Errors) > 0 {
		statuses = append(statuses, posture.Unknown)
	}
	r.Overall = posture.Worst(statuses...)
	return r
}

// unknownPostureRow stands in for a firewall the fan-out never evaluated.
func unknownPostureRow(d fleetDevice, err error) postureRow {
	u := posture.Unknown
	return postureRow{Serial: d.SerialNumber, Name: d.FriendlyName, Tenant: d.TenantID, Online: d.LiveStatus, Overall: u,
		Firmware: d.FirmwareVersion, FirmwareStatus: u, Advisories: make([]string, 0), AdvisoryStatus: u, SupportStatus: u,
		LicenceStatus: u, AdminTOTP: u, SSLVPNZones: make([]string, 0), SSLVPNWAN: u, UserMFA: u,
		Errors: []string{"not checked: " + errText(err)}}
}

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var tenant, serial, dbPath string
	var licenceDays int
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "Report insurance posture as one row of pass/warn/fail/unknown checks per firewall (firmware, support, advisories, licences, admin TOTP, SSL-VPN, user MFA); saved for 'posture changes'",
		Long: "Use this command for a one-row-per-firewall insurance posture summary (firmware, end of support, advisories,\n" +
			"licence expiry, admin TOTP, SSL-VPN exposure) across a tenant or the whole fleet. Do NOT use this command to\n" +
			"find which firewalls one advisory or CVE hits; use 'exposure' instead. Do NOT use it for the per-account list\n" +
			"of users missing one-time passwords; use 'mfa-audit' instead.\n\n" +
			"Each check is pass, warn, fail or unknown. Unknown means the data could not be read (offline firewall,\n" +
			"missing permission, no MySonicWall key); it is never counted as a pass. Licence checks need\n" +
			"SONICWALL_MSW_API_KEY. Each run is saved locally so 'posture changes' can diff runs.",
		Example: strings.Trim(`
  sonicwall-cli posture --agent
  sonicwall-cli posture --serial 2CB8ED000001 --json
  sonicwall-cli posture --licence-days 60 --agent --select serial,overall,advisories`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			devices, err := fetchFleet(ctx, c, tenant, serial)
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			devices, sampled := capForDogfood(devices)
			if len(devices) == 0 {
				// Save the empty run so an emptied fleet shows up in posture changes.
				if dbPath == "" {
					dbPath = defaultDBPath("sonicwall-cli")
				}
				if serr := savePostureSnapshot(ctx, dbPath, time.Now().UTC(), postureScope(tenant, serial), ctx.Err() == nil, nil); serr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: posture snapshot not saved: %v\n", serr)
				}
				return noFirewallsErr(cmd.OutOrStdout(), flags)
			}
			in := postureInputs{mswKey: mswKeyConfigured(flags), licenceWarn: licenceDays,
				supportWindow: 365 * 24 * time.Hour, now: time.Now().UTC()}
			if vulns, verr := fetchVulnerable(ctx, c); verr != nil {
				in.vulnErr = verr
			} else {
				in.vulnerable = map[string][]string{}
				for _, v := range vulns {
					for _, a := range v.advisories() {
						if v.SerialNumber == "" || a.AdvisoryID == "" {
							continue
						}
						key := strings.ToUpper(v.SerialNumber)
						in.vulnerable[key] = append(in.vulnerable[key], a.AdvisoryID)
					}
				}
			}
			results, fails := cliutil.FanoutRun(ctx, devices,
				func(d fleetDevice) string { return d.SerialNumber },
				func(ctx context.Context, d fleetDevice) (postureRow, error) {
					return evaluateDevice(ctx, c, d, in), nil
				})
			bySerial := map[string]postureRow{}
			for _, res := range results {
				bySerial[res.Source] = res.Value
			}
			failErr := map[string]error{}
			for _, f := range fails {
				failErr[f.Source] = f.Err
			}
			rows := make([]postureRow, 0, len(devices))
			unknown := 0
			for _, d := range devices {
				r, done := bySerial[d.SerialNumber]
				if !done {
					err := failErr[d.SerialNumber]
					if err == nil {
						err = fmt.Errorf("no result")
					}
					r = unknownPostureRow(d, err)
				}
				if len(r.Errors) > 0 {
					unknown++
				}
				rows = append(rows, r)
			}
			if unknown > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d firewalls have checks that could not be read (status unknown); see the errors field\n", unknown, len(rows))
			}
			if len(fails) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+fanoutFailureNote(fails, len(devices)))
			}
			{
				if dbPath == "" {
					dbPath = defaultDBPath("sonicwall-cli")
				}
				scope := postureScope(tenant, serial)
				if sampled {
					scope = "dogfood-sample" // a truncated harness sample is never fleet history
				}
				complete := len(fails) == 0 && ctx.Err() == nil && !sampled
				if serr := savePostureSnapshot(ctx, dbPath, in.now, scope, complete, rows); serr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: posture snapshot not saved: %v\n", serr)
				}
			}
			partial := interrupted(ctx)
			if partial == nil && len(fails) > 0 {
				partial = partialFailureErr(fmt.Errorf("%s", fanoutFailureNote(fails, len(devices))))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
					return err
				}
				return partial
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				table = append(table, map[string]any{"serial": r.Serial, "name": r.Name, "overall": r.Overall,
					"firmware": r.FirmwareStatus, "advisories": r.AdvisoryStatus, "support": r.SupportStatus,
					"licences": r.LicenceStatus, "admin_totp": r.AdminTOTP, "sslvpn_wan": r.SSLVPNWAN, "user_mfa": r.UserMFA})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"serial", "name", "overall", "firmware", "advisories", "support", "licences", "admin_totp", "sslvpn_wan", "user_mfa"}, table); err != nil {
				return err
			}
			return partial
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Only firewalls in this NSM tenant ID (tenantId from 'devices list')")
	cmd.Flags().StringVar(&serial, "serial", "", "Only this firewall serial number")
	cmd.Flags().IntVar(&licenceDays, "licence-days", 30, "Warn when the soonest licence expires within this many days")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for posture snapshots (default: the CLI's local store)")
	addNovelCommandIfAbsent(cmd, newNovelPostureChangesCmd(flags))
	return cmd
}

// postureScope names what a posture run covered, so 'posture changes' only
// compares runs over the same set of firewalls. Both filters are part of the
// scope, so a --serial run with a mismatched --tenant is never compared with
// (or empties) the plain --serial history. Tenant and serial are case-folded
// because the fleet filter matches them without regard to case.
func postureScope(tenant, serial string) string {
	switch {
	case serial != "" && tenant != "":
		return "tenant:" + strings.ToLower(tenant) + "/serial:" + strings.ToUpper(serial)
	case serial != "":
		return "serial:" + strings.ToUpper(serial)
	case tenant != "":
		return "tenant:" + strings.ToLower(tenant)
	}
	return "fleet"
}
