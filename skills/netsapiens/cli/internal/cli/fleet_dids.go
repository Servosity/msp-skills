// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type didRoute struct {
	PhoneNumber string   `json:"phonenumber"`
	Domain      string   `json:"domain"`
	Enabled     *bool    `json:"enabled"` // nil when the record omits the field
	Application string   `json:"application"`
	DestUser    string   `json:"destination_user,omitempty"`
	DestHost    string   `json:"destination_host,omitempty"`
	TargetType  string   `json:"target_type"` // user | callqueue | autoattendant | device | external | parked | none
	TargetName  string   `json:"target_name,omitempty"`
	Problems    []string `json:"problems"`
	Description string   `json:"description,omitempty"`
}

// didDestKind groups the v45 DialruleApplication values by what the destination names.
func didDestKind(app string) string {
	switch {
	case app == "available-number" || app == "hangup":
		return "parked"
	case app == "toggle-timeframe":
		return "none"
	case strings.HasPrefix(app, "to-connection"):
		return "external"
	case strings.HasPrefix(app, "to-callqueue"):
		return "callqueue"
	case strings.HasPrefix(app, "to-voicemail"):
		return "voicemail"
	case strings.HasPrefix(app, "to-single-device"):
		return "device"
	default: // to-user*, spam-screening, unknown future apps that ring a user
		return "user"
	}
}

// didCoverage says which target tables were synced; an unsynced table makes a
// target unverifiable, not missing.
type didCoverage struct{ domains, users, callqueues, autoattendants, devices bool }

func auditDIDs(numbers, domains, users, queues, attendants, devices []fleetRow, cov didCoverage) []didRoute {
	synced := map[string]bool{}
	for _, d := range domains {
		synced[d.fstr("domain")] = true
	}
	byUser, byQueue := indexBy(users, "user"), indexBy(queues, "callqueue")
	byAA, byDevice := indexBy(attendants, "user"), indexBy(devices, "device")
	reg := registeredDevices(devices)

	out := make([]didRoute, 0, len(numbers))
	seen := map[string]bool{}
	for _, n := range numbers {
		num := n.fstr("phonenumber")
		if num == "" || seen[num] {
			continue
		}
		seen[num] = true
		r := didRoute{
			PhoneNumber: num, Domain: n.domainOf(), Enabled: optYes(n, "enabled"),
			Application: n.fstr("dial-rule-application"),
			DestUser:    n.fstr("dial-rule-translation-destination-user"),
			DestHost:    n.fstr("dial-rule-translation-destination-host"),
			Description: n.fstr("dial-rule-description"),
			Problems:    []string{},
		}
		if r.Enabled != nil && !*r.Enabled {
			r.Problems = append(r.Problems, "disabled")
		}
		if !has(n, "dial-rule-application") {
			r.TargetType = "unverified"
			r.Problems = append(r.Problems, "routing-unverified:application-field-absent")
			out = append(out, r)
			continue
		}
		kind := didDestKind(r.Application)
		dest := r.DestHost
		if dest == "" || strings.Contains(dest, "*") || strings.Contains(dest, "<") {
			dest = r.Domain // "[*]" / "<OwnDomain>" style placeholders mean the number's own domain
		}
		hostKnown := has(n, "dial-rule-translation-destination-host")
		switch kind {
		case "parked":
			r.TargetType = "parked"
			r.Problems = append(r.Problems, "parked:"+r.Application)
		case "none":
			r.TargetType = "none"
		case "external":
			r.TargetType = "external"
		default:
			if !has(n, "dial-rule-translation-destination-user") || !hostKnown {
				r.TargetType = "unverified"
				r.Problems = append(r.Problems, "routing-unverified:destination-field-absent") // user or host field omitted
				break
			}
			if r.DestUser == "" {
				r.TargetType = "none"
				r.Problems = append(r.Problems, "no-destination")
				break
			}
			if dest == "" { // neither the number's domain nor a destination host is known
				r.TargetType = "unverified"
				r.Problems = append(r.Problems, "routing-unverified:number-domain-absent")
				break
			}
			if !synced[dest] {
				if cov.domains { // a completed domains sync without it: the destination domain does not exist
					r.TargetType = "missing"
					r.Problems = append(r.Problems, "dangling:domain-"+dest+"-not-found")
				} else {
					r.TargetType = "unverified"
					r.Problems = append(r.Problems, "destination-domain-not-synced")
				}
				break
			}
			k := key(dest, r.DestUser)
			// The application decides which table the destination must exist in: a
			// to-callqueue route at extension 700 is dangling when only user 700 exists.
			type cand struct {
				typ     string
				row     fleetRow
				covered bool
			}
			var cands []cand
			switch kind {
			case "callqueue":
				cands = []cand{{"callqueue", byQueue[k], cov.callqueues}}
			case "device":
				cands = []cand{{"device", byDevice[k], cov.devices}}
			case "voicemail":
				cands = []cand{{"user", byUser[k], cov.users}}
			default: // user-ringing applications may target a user or an auto attendant
				cands = []cand{{"autoattendant", byAA[k], cov.autoattendants}, {"user", byUser[k], cov.users}}
			}
			var hit *cand
			unverified := false
			for i := range cands {
				if cands[i].row != nil {
					hit = &cands[i]
					break
				}
				if !cands[i].covered {
					unverified = true
				}
			}
			switch {
			case hit != nil:
				r.TargetType = hit.typ
				switch hit.typ {
				case "callqueue":
					r.TargetName = hit.row.fstr("description")
				case "autoattendant":
					r.TargetName = hit.row.fstr("attendant-name")
				case "user":
					r.TargetName = fullName(hit.row)
					// a confirmed dead end needs every device state known and voicemail explicitly off
					if kind == "user" && cov.devices && reg.forUser(dest, r.DestUser).knownZero() && has(hit.row, "voicemail-enabled") && !hit.row.fyes("voicemail-enabled") {
						r.Problems = append(r.Problems, "possible-dead-end:no-registered-device-and-voicemail-off")
					}
				}
			case unverified:
				r.TargetType = "unverified"
				r.Problems = append(r.Problems, "target-table-not-synced:"+cands[len(cands)-1].typ)
			default:
				r.TargetType = "missing"
				r.Problems = append(r.Problems, fmt.Sprintf("dangling:no-%s-%s@%s", cands[len(cands)-1].typ, r.DestUser, dest))
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (len(out[i].Problems) > 0) != (len(out[j].Problems) > 0) {
			return len(out[i].Problems) > 0
		}
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].PhoneNumber < out[j].PhoneNumber
	})
	return out
}

// didReport is the machine result: the routes plus whether the number table
// behind them was fully synced.
type didReport struct {
	auditCoverage
	Numbers []didRoute `json:"numbers"`
}

func newNovelFleetDidsCmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain string
	var problemsOnly bool
	cmd := &cobra.Command{
		Use:   "dids",
		Short: "Where every phone number routes, flagging numbers that point at missing or unreachable targets",
		Long: `Use this command for a fleet-wide audit of where every DID routes and which DIDs route nowhere.
Do NOT use this command to resolve a single number to its owner and phones; use 'trace' instead.

Each number's dial-rule application and destination are checked against the synced users,
call queues, auto attendants and devices of the destination domain. Answer rules are not
inspected, so a user with no registered phone and voicemail off is reported as a possible dead end.`,
		Example: strings.Trim(`
  netsapiens-cli fleet dids --problems-only --agent
  netsapiens-cli fleet dids --select numbers.phonenumber,numbers.problems
  netsapiens-cli fleet dids --domain acme.example
  netsapiens-cli fleet dids --csv --select phonenumber,domain,application,target_type,problems`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet dids")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(ctx, cmd, flags, dbPath, didReport{auditCoverage: noMirrorCoverage(), Numbers: []didRoute{}})
			if db == nil || err != nil {
				return err
			}
			defer db.Close()
			hintCoverage(cmd, db, rtPhonenumbers, rtDomains)
			cov := fleetCov{}
			var sets [6][]fleetRow
			for i, rt := range []string{rtPhonenumbers, rtDomains, rtUsers, rtCallqueues, rtAutoattendants, rtDevices} {
				if sets[i], err = loadCovered(ctx, db, cov, rt); err != nil {
					return err
				}
			}
			routes := auditDIDs(filterDomain(sets[0], domain), sets[1], sets[2], sets[3], sets[4], sets[5],
				didCoverage{domains: cov[rtDomains], users: cov[rtUsers], callqueues: cov[rtCallqueues], autoattendants: cov[rtAutoattendants], devices: cov[rtDevices]})
			if problemsOnly {
				kept := routes[:0]
				for _, r := range routes {
					if len(r.Problems) > 0 {
						kept = append(kept, r)
					}
				}
				routes = kept
			}
			// Per-target coverage is already in each route (unverified targets); the
			// number list itself must come from a covered phonenumbers walk.
			ac := fleetAuditCoverage(cmd, flags, db, cov, []string{rtPhonenumbers})
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// keep: --agent/--compact drop keys present on few rows; these show where a number points.
				return printFleetAudit(cmd, flags, ac, didReport{auditCoverage: ac, Numbers: routes}, routes, "destination_user", "destination_host", "target_name")
			}
			printIncomplete(cmd, ac)
			if len(routes) == 0 {
				if ac.Complete {
					fmt.Fprintln(cmd.OutOrStdout(), "No matching phone numbers in the local mirror.")
				}
				return nil
			}
			table := make([]map[string]any, 0, len(routes))
			for _, r := range routes {
				table = append(table, map[string]any{"number": r.PhoneNumber, "domain": r.Domain, "app": r.Application, "to": r.DestUser, "target": r.TargetType, "problems": strings.Join(r.Problems, ",")})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only numbers owned by this client domain")
	cmd.Flags().BoolVar(&problemsOnly, "problems-only", false, "Only numbers with a routing problem")
	return cmd
}
