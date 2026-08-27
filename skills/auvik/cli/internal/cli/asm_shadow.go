// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
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
	Apps     int             `json:"applications"`
	Users    int             `json:"users"`
	Licenses int             `json:"licenses"`
	Counts   map[string]int  `json:"counts"`
	Findings []shadowFinding `json:"findings"`
	Note     string          `json:"note,omitempty"`
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
Set-joins the locally synced Auvik SaaS Management applications, users, and
licenses per ASM client, then reports two findings:

  unlicensed_usage  an application has active users but no license records
  unused_licenses   licenses exist that no active user is consuming

The ASM application, user, and license records are three separate endpoints;
the set-join across them is what turns them into an entitlement finding.

Use this command for SaaS application, user, and license inventory from Auvik
SaaS Management. Do NOT use this command for Auvik's own billable network-device
counts; use 'usage reconcile' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources asm,asm-user-info,asm-license-info,asm-client-info --full
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

			if !hintIfUnsynced(cmd, db, "asm") {
				hintIfStale(cmd, db, "asm", flags.maxAge)
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
			licenses, err := loadResources(ctx, db, rtASMLicenses)
			if err != nil {
				return err
			}
			clients, err := loadResources(ctx, db, rtASMClients)
			if err != nil {
				return err
			}

			report := buildShadowReport(apps, users, licenses, clients)

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
				report.Note = "no ASM applications in the local mirror; run 'auvik-cli sync --resources asm,asm-user-info,asm-license-info --full'. Auvik SaaS Management is a separate product tier -- if your tenant does not license it, these endpoints return nothing."
			}

			done, err := emitLocalReport(cmd, flags, report, report.Findings)
			if err != nil || done {
				return err
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "SaaS license findings across %d application(s), %d user(s), %d license record(s)\n",
				report.Apps, report.Users, report.Licenses)
			fmt.Fprintf(w, "  unlicensed usage: %d   unused licenses: %d\n\n",
				report.Counts[findingUnlicensed], report.Counts[findingUnusedSeat])
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
//   - asmLicense*:          NO relationships at all -- licenses carry an `email`
//     and join to users by that address, not to apps.
//
// So the app is the anchor: each app names its users, each user has an email,
// and licenses are matched into an app through the emails of that app's users.
func buildShadowReport(apps, users, licenses, clients []auvikRow) shadowReport {
	report := shadowReport{
		Apps:     len(apps),
		Users:    len(users),
		Licenses: len(licenses),
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

	// user id -> email, plus whether the user is actually consuming the app.
	type userFacts struct {
		email  string
		active bool
	}
	userByID := map[string]userFacts{}
	emailToUser := map[string]string{}
	for _, u := range users {
		email := strings.ToLower(strings.TrimSpace(u.attrString(fASMUserEmail.Field)))
		userByID[u.ID] = userFacts{email: email, active: asmUserActive(u)}
		if email != "" {
			emailToUser[email] = u.ID
		}
	}

	// license email -> license rows (a user may hold several license types).
	licensesByEmail := map[string][]auvikRow{}
	for _, l := range licenses {
		email := strings.ToLower(strings.TrimSpace(l.attrString(fASMLicenseEmail.Field)))
		if email == "" {
			continue
		}
		licensesByEmail[email] = append(licensesByEmail[email], l)
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

	licensedEmailsSeen := map[string]bool{}

	for _, app := range ordered {
		appName := firstNonEmpty(app.attrString(fASMAppName.Field), app.ID)
		clientID := app.rel(rASMAppClient.Name)

		activeUsers := 0
		licensedHere := 0
		for uid := range appUsers[app.ID] {
			uf, ok := userByID[uid]
			if !ok {
				continue
			}
			if uf.active {
				activeUsers++
			}
			if uf.email != "" {
				if rows, has := licensesByEmail[uf.email]; has {
					licensedHere += len(rows)
					licensedEmailsSeen[uf.email] = true
				}
			}
		}

		switch {
		case activeUsers > 0 && licensedHere == 0:
			report.Findings = append(report.Findings, shadowFinding{
				Finding: findingUnlicensed, Client: label(clientID), Application: appName, AppID: app.ID,
				ActiveUsers: activeUsers, Licenses: 0,
				Detail: fmt.Sprintf("%d active user(s), no license record", activeUsers),
			})
			report.Counts[findingUnlicensed]++
		case licensedHere > activeUsers:
			report.Findings = append(report.Findings, shadowFinding{
				Finding: findingUnusedSeat, Client: label(clientID), Application: appName, AppID: app.ID,
				ActiveUsers: activeUsers, Licenses: licensedHere,
				Detail: fmt.Sprintf("%d license(s) beyond active usage", licensedHere-activeUsers),
			})
			report.Counts[findingUnusedSeat]++
		case activeUsers > licensedHere && licensedHere > 0:
			report.Findings = append(report.Findings, shadowFinding{
				Finding: findingUnlicensed, Client: label(clientID), Application: appName, AppID: app.ID,
				ActiveUsers: activeUsers, Licenses: licensedHere,
				Detail: fmt.Sprintf("%d active user(s) beyond licensed seats", activeUsers-licensedHere),
			})
			report.Counts[findingUnlicensed]++
		}
	}

	// Licenses whose email matches no active user of any app: pure waste.
	orphanByClient := map[string]int{}
	for email, rows := range licensesByEmail {
		if licensedEmailsSeen[email] {
			continue
		}
		uid, known := emailToUser[email]
		if known && userByID[uid].active {
			continue
		}
		orphanByClient["(unattributed)"] += len(rows)
	}
	for client, n := range orphanByClient {
		report.Findings = append(report.Findings, shadowFinding{
			Finding: findingUnusedSeat, Client: client, Application: "(no matching active user)",
			ActiveUsers: 0, Licenses: n,
			Detail: fmt.Sprintf("%d license(s) assigned to an address with no active ASM user", n),
		})
		report.Counts[findingUnusedSeat]++
	}

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
