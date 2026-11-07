// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"auvik-pp-cli/internal/client"
)

type shadowFinding struct {
	Finding     string `json:"finding"`
	Client      string `json:"client"`
	Application string `json:"application"`
	AppID       string `json:"app_id,omitempty"`
	ActiveUsers int    `json:"active_users"`
	Licenses    int    `json:"licenses"`
	Detail      string `json:"detail"`
}

type shadowReport struct {
	Apps     int `json:"applications"`
	Users    int `json:"users"`
	Licenses int `json:"licenses"`
	// UnattributedLicenses counts license rows that name no application and
	// were therefore not counted for any (see buildShadowReport).
	UnattributedLicenses int `json:"unattributed_licenses,omitempty"`
	// LicenseLookupFailed lists "<app>: <reason>" for applications whose
	// license read failed; they carry no finding either way.
	LicenseLookupFailed []string        `json:"license_lookup_failed,omitempty"`
	Counts              map[string]int  `json:"counts"`
	Findings            []shadowFinding `json:"findings"`
	Note                string          `json:"note,omitempty"`
}

const (
	findingUnlicensed = "unlicensed_usage"
	findingUnusedSeat = "unused_licenses"
)

func newNovelAsmShadowCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		client  string
		finding string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "shadow",
		Short: "Surface SaaS apps with active users but no license record, and licenses nobody is using, per client.",
		Long: strings.Trim(`
Joins the locally synced Auvik SaaS Management applications and users per ASM
client with each application's own license records, then reports two findings:

  unlicensed_usage  an application has active users with no license record for it
  unused_licenses   an application has licenses no active user of it holds

Auvik's license record does not name its application; the license endpoint
returns one application's licenses at a time. So licenses are read from the API
per application (one read per synced application, scoped by its ASM client),
never from the mirror, and a license is only ever counted for the application
it was returned for. An application whose license read fails is listed under
license_lookup_failed and gets no finding.

Use this command for SaaS application, user, and license inventory from Auvik
SaaS Management. Do NOT use this command for Auvik's own billable network-device
counts; use 'usage reconcile' instead.

Reads applications and users from the local mirror. The ASM application and
user endpoints require an ASM client id, so sync the clients first, then each
client's applications and users:
  auvik-cli sync --resources asm --full
  auvik-cli sync --resources asm-app-info,asm-user-info --param filter[clientId]=<asm-client-id> --full
`, "\n"),
		Example: strings.Trim(`
  # Everything worth a spend conversation
  auvik-cli asm shadow --agent

  # Only apps being used with no license behind them
  auvik-cli asm shadow --finding unlicensed_usage --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "asm shadow")
			}
			if finding != "" && finding != findingUnlicensed && finding != findingUnusedSeat {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--finding must be one of %s, %s", findingUnlicensed, findingUnusedSeat))
			}

			empty := shadowReport{Counts: map[string]int{}, Findings: []shadowFinding{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtASMApps.current()) {
				hintIfStale(cmd, db, rtASMApps.current(), flags.maxAge)
			}

			ctx := cmd.Context()
			apps, err := loadResources(ctx, db, rtASMApps)
			if err != nil {
				return err
			}
			users, err := loadResources(ctx, db, rtASMUsers)
			if err != nil {
				return err
			}
			clients, err := loadResources(ctx, db, rtASMClients)
			if err != nil {
				return err
			}
			// Licenses are read per application from the API, never from the
			// mirror: the license record does not name its application, so
			// only the request scope binds a license to the app it is for.
			var licenses []auvikRow
			lookupFailed := map[string]string{}
			if len(apps) > 0 {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				licenses, lookupFailed = fetchASMLicenses(ctx, c, apps)
			}

			report := buildShadowReport(apps, users, licenses, clients, lookupFailed)

			out := report.Findings
			if finding != "" {
				filtered := make([]shadowFinding, 0, len(out))
				for _, f := range out {
					if f.Finding == finding {
						filtered = append(filtered, f)
					}
				}
				out = filtered
			}
			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]shadowFinding, 0, len(out))
				for _, f := range out {
					if strings.Contains(strings.ToLower(f.Client), needle) {
						filtered = append(filtered, f)
					}
				}
				out = filtered
			}
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			report.Findings = out

			if report.Apps == 0 {
				report.Note = "no ASM applications in the local mirror; sync ASM clients, then each client's applications and users (see 'auvik-cli asm shadow --help'). Auvik SaaS Management is a separate product tier -- if your tenant does not license it, these endpoints return nothing."
			} else if len(report.LicenseLookupFailed) > 0 {
				report.Note = fmt.Sprintf("%d application(s) carry no finding because their license read failed; see license_lookup_failed", len(report.LicenseLookupFailed))
			}

			done, err := emitLocalReport(cmd, flags, report, report.Findings)
			if err != nil || done {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "SaaS license findings across %d application(s), %d user(s), %d license record(s)\n",
				report.Apps, report.Users, report.Licenses)
			fmt.Fprintf(w, "  unlicensed usage: %d   unused licenses: %d\n",
				report.Counts[findingUnlicensed], report.Counts[findingUnusedSeat])
			if len(report.LicenseLookupFailed) > 0 {
				fmt.Fprintf(w, "  license read failed for %d application(s); they carry no finding: %s\n",
					len(report.LicenseLookupFailed), strings.Join(report.LicenseLookupFailed, "; "))
			}
			fmt.Fprintln(w)
			if len(report.Findings) == 0 {
				if report.Note != "" {
					fmt.Fprintln(w, report.Note)
				} else {
					fmt.Fprintln(w, "No findings match the given filters.")
				}
				return nil
			}
			fmt.Fprintf(w, "%-18s %-22s %-28s %6s %9s  %s\n",
				"FINDING", "CLIENT", "APPLICATION", "USERS", "LICENSES", "DETAIL")
			for _, f := range report.Findings {
				fmt.Fprintf(w, "%-18s %-22s %-28s %6d %9d  %s\n",
					f.Finding, truncate(f.Client, 22), truncate(f.Application, 28),
					f.ActiveUsers, f.Licenses, f.Detail)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&client, "client", "", "Only show findings whose client name contains this substring")
	cmd.Flags().StringVar(&finding, "finding", "", "Only one finding kind: unlicensed_usage, unused_licenses")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum findings to return (0 = all)")
	return cmd
}

// buildShadowReport is the pure set-join core, split out for table tests.
//
// FIELD SOURCING: the relationship shape is not symmetric.
//   - asmAppRelationships:  breaches, client, contracts, users  (users is a LIST)
//   - asmUserRelationships: applications                        (a LIST)
//   - asmLicense*:          NO relationships and no application attribute --
//     a license carries an `email`, and the application it belongs to is known
//     only from the request that returned it (/v1/asm/license/info REQUIRES
//     filter[applicationId]).
//
// So the app is the anchor: each app names its users, each user has an email,
// and an app's OWN licenses (rows tagged with asmLicenseAppKey by
// fetchASMLicenses) are matched against the emails of that app's active users.
// A license row with no application tag -- every row in the local mirror -- is
// never counted for any application: counting it against every app its holder
// uses would mark an app licensed by another app's seat.
//
// lookupFailed names the apps whose license read failed; they get no finding
// either way, because "no license record" was never observed for them.
func buildShadowReport(apps, users, licenses, clients []auvikRow, lookupFailed map[string]string) shadowReport {
	report := shadowReport{
		Apps:     len(apps),
		Users:    len(users),
		Counts:   map[string]int{findingUnlicensed: 0, findingUnusedSeat: 0},
		Findings: make([]shadowFinding, 0),
	}

	clientNames := map[string]string{}
	for _, c := range clients {
		clientNames[c.ID] = firstNonEmpty(c.attrString(fASMClientName.Field), c.ID)
	}
	label := func(id string) string {
		if n, ok := clientNames[id]; ok && n != "" {
			return n
		}
		if id == "" {
			return "(unknown)"
		}
		return id
	}

	type userFacts struct {
		email  string
		active bool
	}
	userByID := map[string]userFacts{}
	for _, u := range users {
		email := strings.ToLower(strings.TrimSpace(u.attrString(fASMUserEmail.Field)))
		userByID[u.ID] = userFacts{email: email, active: asmUserActive(u)}
	}

	// Each application's own license emails, from tagged rows only.
	licenseEmailsByApp := map[string][]string{}
	for _, l := range licenses {
		appID, _ := l.Data[asmLicenseAppKey].(string)
		if appID == "" {
			report.UnattributedLicenses++
			continue
		}
		report.Licenses++
		licenseEmailsByApp[appID] = append(licenseEmailsByApp[appID],
			strings.ToLower(strings.TrimSpace(l.attrString(fASMLicenseEmail.Field))))
	}

	// Apps that never appear on any user's `applications` list still matter, so
	// build the app->users edge from both directions.
	appUsers := map[string]map[string]bool{}
	for _, a := range apps {
		set := map[string]bool{}
		for _, uid := range a.relMany(rASMAppUsers.Name) {
			set[uid] = true
		}
		appUsers[a.ID] = set
	}
	for _, u := range users {
		for _, appID := range u.relMany(rASMUserApps.Name) {
			if appUsers[appID] == nil {
				appUsers[appID] = map[string]bool{}
			}
			appUsers[appID][u.ID] = true
		}
	}

	ordered := make([]auvikRow, len(apps))
	copy(ordered, apps)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	for _, app := range ordered {
		appName := firstNonEmpty(app.attrString(fASMAppName.Field), app.ID)
		clientID := app.rel(rASMAppClient.Name)
		if reason, failed := lookupFailed[app.ID]; failed {
			report.LicenseLookupFailed = append(report.LicenseLookupFailed, appName+": "+reason)
			continue
		}

		licensed := map[string]bool{}
		for _, email := range licenseEmailsByApp[app.ID] {
			if email != "" {
				licensed[email] = true
			}
		}
		activeEmails := map[string]bool{}
		activeUsers, unlicensedActive := 0, 0
		for uid := range appUsers[app.ID] {
			uf, ok := userByID[uid]
			if !ok || !uf.active {
				continue
			}
			activeUsers++
			if uf.email == "" {
				// No address to match a license on: neither licensed nor
				// provably unlicensed.
				continue
			}
			activeEmails[uf.email] = true
			if !licensed[uf.email] {
				unlicensedActive++
			}
		}
		appLicenses := len(licenseEmailsByApp[app.ID])
		unused := 0
		for _, email := range licenseEmailsByApp[app.ID] {
			if email == "" || !activeEmails[email] {
				unused++
			}
		}

		if unlicensedActive > 0 {
			report.Findings = append(report.Findings, shadowFinding{
				Finding: findingUnlicensed, Client: label(clientID), Application: appName, AppID: app.ID,
				ActiveUsers: activeUsers, Licenses: appLicenses,
				Detail: fmt.Sprintf("%d active user(s) with no license record for this application", unlicensedActive),
			})
			report.Counts[findingUnlicensed]++
		}
		if unused > 0 {
			report.Findings = append(report.Findings, shadowFinding{
				Finding: findingUnusedSeat, Client: label(clientID), Application: appName, AppID: app.ID,
				ActiveUsers: activeUsers, Licenses: appLicenses,
				Detail: fmt.Sprintf("%d license(s) not held by an active user of this application", unused),
			})
			report.Counts[findingUnusedSeat]++
		}
	}
	sort.Strings(report.LicenseLookupFailed)

	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		if a.Finding != b.Finding {
			return a.Finding < b.Finding
		}
		return a.Application < b.Application
	})
	return report
}

// asmLicenseAppKey tags a license row with the application its request was
// scoped to. It sits beside the JSON:API members, never inside attributes, so
// it cannot be mistaken for a vendor field.
const asmLicenseAppKey = "x-auvik-pp-application-id"

// asmLicenseMaxPages bounds one application's license walk.
const asmLicenseMaxPages = 100

// fetchASMLicenses reads each application's own license records from
// /v1/asm/license/info, scoped by filter[clientId] and filter[applicationId]
// (both REQUIRED by the endpoint), and tags every row with that application.
// An application whose read fails is returned in failed with a reason that
// names the HTTP status only -- a response body is never echoed. Once the API
// rejects the credential (401/403) the remaining reads are not attempted.
func fetchASMLicenses(ctx context.Context, c *client.Client, apps []auvikRow) (licenses []auvikRow, failed map[string]string) {
	failed = map[string]string{}
	authFailure := ""
	for _, app := range apps {
		if authFailure != "" {
			failed[app.ID] = authFailure
			continue
		}
		clientID := app.rel(rASMAppClient.Name)
		if clientID == "" {
			failed[app.ID] = "application names no ASM client to scope the license read"
			continue
		}
		rows, err := fetchAppLicenses(ctx, c, clientID, app.ID)
		if err != nil {
			reason := "license read failed"
			var apiErr *client.APIError
			switch {
			case errors.As(err, &apiErr):
				reason = fmt.Sprintf("license read returned HTTP %d", apiErr.StatusCode)
				if apiErr.StatusCode == 401 || apiErr.StatusCode == 403 {
					authFailure = reason
				}
			case errors.Is(err, errASMLicensePaging):
				reason = err.Error()
			}
			failed[app.ID] = reason
			continue
		}
		licenses = append(licenses, rows...)
	}
	return licenses, failed
}

var errASMLicensePaging = errors.New("license read stopped: pagination did not complete")

// fetchAppLicenses walks one (client, application) license list. Pagination
// re-issues the request against the configured API host with the page[after]
// cursor taken from links.next; the link's own scheme and host are never
// dialed, so the credential cannot follow a link elsewhere.
func fetchAppLicenses(ctx context.Context, c *client.Client, clientID, appID string) ([]auvikRow, error) {
	params := map[string]string{"filter[clientId]": clientID, "filter[applicationId]": appID}
	seen := map[string]bool{}
	var out []auvikRow
	for page := 0; page < asmLicenseMaxPages; page++ {
		raw, err := c.Get(ctx, "/v1/asm/license/info", params)
		if err != nil {
			return nil, err
		}
		var env struct {
			Data  []json.RawMessage `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(raw, &env); err != nil || env.Data == nil {
			return nil, fmt.Errorf("%w (page carries no data member)", errASMLicensePaging)
		}
		for _, item := range env.Data {
			var obj map[string]any
			if json.Unmarshal(item, &obj) != nil || anyString(obj["type"]) != "asmLicense" {
				continue
			}
			obj[asmLicenseAppKey] = appID
			out = append(out, auvikRow{ID: anyString(obj["id"]), Data: obj})
		}
		next := strings.TrimSpace(env.Links.Next)
		if next == "" {
			return out, nil
		}
		u, err := url.Parse(next)
		cursor := ""
		if err == nil {
			cursor = u.Query().Get("page[after]")
		}
		if cursor == "" || seen[cursor] {
			return nil, fmt.Errorf("%w (next link carries no new page[after] cursor)", errASMLicensePaging)
		}
		seen[cursor] = true
		params["page[after]"] = cursor
	}
	return nil, fmt.Errorf("%w (more than %d pages)", errASMLicensePaging, asmLicenseMaxPages)
}

// asmUserActive treats a user as consuming the app unless the record explicitly
// says otherwise. Auvik marks deprovisioned users via status/active fields.
func asmUserActive(u auvikRow) bool {
	// asmUserAttributes carries both `active` and `disabled`.
	if v := u.attr(fASMUserDisabled.Field); v != nil && truthy(v) {
		return false
	}
	if v := u.attr(fASMUserActive.Field); v != nil {
		return truthy(v)
	}
	return true
}
