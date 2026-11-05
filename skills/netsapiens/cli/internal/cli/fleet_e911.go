// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type e911Gap struct {
	Domain       string   `json:"domain"`
	Entity       string   `json:"entity"` // domain | user | device
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	User         string   `json:"user,omitempty"` // device rows: the owning extension
	Gaps         []string `json:"gaps"`
	EffectiveCID string   `json:"effective_emergency_caller_id,omitempty"`
	AddressID    string   `json:"emergency_address_id,omitempty"`
}

type e911Report struct {
	DomainsChecked int       `json:"domains_checked"`
	UsersChecked   int       `json:"users_checked"`
	DevicesChecked int       `json:"devices_checked"`
	AddressesKnown int       `json:"addresses_known"`
	Gaps           []e911Gap `json:"gaps"`
	Note           string    `json:"note,omitempty"`
	auditCoverage
}

// e911Gaps resolves the emergency caller ID through the NetSapiens fallback
// chain (device -> user -> domain) and checks every emergency-address-id
// against the addresses synced for that domain.
// e911Coverage says which inheritance levels completed their latest sync.
type e911Coverage struct{ domains, users, devices, addresses bool }

// cidState is one inheritance level's emergency caller ID.
type cidState int

const (
	cidUnknown cidState = iota // row or field not available: no claim possible
	cidUnset                   // field present but empty / "[*]" (inherit)
	cidSet
)

func cidOf(r fleetRow) (string, cidState) {
	if r == nil || !has(r, "caller-id-number-emergency") {
		return "", cidUnknown
	}
	if v := realValue(r.fstr("caller-id-number-emergency")); v != "" {
		return v, cidSet
	}
	return "", cidUnset
}

// resolveCID walks the device -> user -> domain chain. Any set level means the
// entity has an emergency caller ID (no gap); otherwise any unknown level makes
// the answer unverified; only an all-unset chain is a gap.
func resolveCID(levels ...cidState) cidState {
	unknown := false
	for _, l := range levels {
		switch l {
		case cidSet:
			return cidSet
		case cidUnknown:
			unknown = true
		}
	}
	if unknown {
		return cidUnknown
	}
	return cidUnset
}

// e911Gaps resolves each entity's emergency caller ID through the NetSapiens
// fallback chain and checks emergency-address-id references against the
// addresses synced for that domain. Inputs come from completed syncs only.
func e911Gaps(domains, users, devices, addresses []fleetRow, cov e911Coverage) e911Report {
	rep := e911Report{DomainsChecked: len(domains), UsersChecked: len(users), DevicesChecked: len(devices), AddressesKnown: len(addresses), Gaps: []e911Gap{}}
	addr := map[string]bool{}
	for _, a := range addresses {
		if id := a.fstr("emergency-address-id"); id != "" {
			addr[key(a.domainOf(), id)] = true
		}
	}
	domBy := map[string]fleetRow{}
	for _, d := range domains {
		domBy[d.fstr("domain")] = d
	}
	userBy := indexBy(users, "user")
	// level returns a row's state; a row absent from a completed table is unset
	// (the entity has no record there), absent from an incomplete one is unknown.
	level := func(r fleetRow, covered bool) cidState {
		if r == nil {
			if covered {
				return cidUnset
			}
			return cidUnknown
		}
		_, st := cidOf(r)
		return st
	}
	checkAddr := func(r fleetRow, dom string) []string {
		id := realValue(r.fstr("emergency-address-id"))
		if id == "" || dom == "" || !cov.addresses || addr[key(dom, id)] { // no tenant: cannot be judged
			return nil
		}
		return []string{"emergency-address-id-not-found"}
	}
	add := func(g e911Gap, st cidState, unsetGap, unknownGap string, extra []string) {
		switch st {
		case cidUnset:
			g.Gaps = append(g.Gaps, unsetGap)
		case cidUnknown:
			g.Gaps = append(g.Gaps, unknownGap)
		}
		g.Gaps = append(g.Gaps, extra...)
		if len(g.Gaps) > 0 {
			rep.Gaps = append(rep.Gaps, g)
		}
	}
	for _, d := range domains {
		name := d.fstr("domain")
		add(e911Gap{Domain: name, Entity: "domain", ID: name, Name: d.fstr("description"), AddressID: d.fstr("emergency-address-id")},
			level(d, true), "no-domain-emergency-caller-id", "emergency-caller-id-unverified:field-absent", checkAddr(d, name))
	}
	for _, u := range users {
		dom, id := u.domainOf(), u.fstr("user")
		domLevel := level(domBy[dom], cov.domains)
		if dom == "" {
			domLevel = cidUnknown // no tenant to inherit from
		}
		st := resolveCID(level(u, true), domLevel)
		add(e911Gap{Domain: dom, Entity: "user", ID: id, Name: fullName(u), AddressID: u.fstr("emergency-address-id")},
			st, "no-emergency-caller-id-after-domain-fallback", "emergency-caller-id-unverified", checkAddr(u, dom))
	}
	for _, dv := range devices {
		dom, user := dv.domainOf(), dv.fstr("user")
		userLevel := level(userBy[key(dom, user)], cov.users)
		domLevel := level(domBy[dom], cov.domains)
		if !has(dv, "user") || dom == "" { // owner or tenant unknown: inherited levels cannot be resolved
			userLevel = cidUnknown
		}
		if dom == "" {
			domLevel = cidUnknown
		}
		st := resolveCID(level(dv, true), userLevel, domLevel)
		add(e911Gap{Domain: dom, Entity: "device", ID: dv.fstr("device"), User: user, AddressID: dv.fstr("emergency-address-id")},
			st, "no-emergency-caller-id-after-user-and-domain-fallback", "emergency-caller-id-unverified", checkAddr(dv, dom))
	}
	sort.SliceStable(rep.Gaps, func(i, j int) bool {
		a, b := rep.Gaps[i], rep.Gaps[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Entity != b.Entity { // domain-level gaps first: one fix covers every user and device under it
			return e911EntityRank[a.Entity] < e911EntityRank[b.Entity]
		}
		return a.ID < b.ID
	})
	var notes []string
	if !cov.addresses {
		notes = append(notes, "emergency addresses are not fully synced, so address ids were not validated; run: netsapiens-cli sync --resources domains_addresses")
	}
	for name, ok := range map[string]bool{"domains": cov.domains, "users": cov.users, "devices": cov.devices} {
		if !ok {
			notes = append(notes, name+" did not complete a sync and were not checked")
		}
	}
	sort.Strings(notes)
	rep.Note = strings.Join(notes, "; ")
	return rep
}

var e911EntityRank = map[string]int{"domain": 0, "user": 1, "device": 2}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func newNovelFleetE911Cmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain string
	cmd := &cobra.Command{
		Use:   "e911",
		Short: "Domains, users and devices missing an emergency caller ID or pointing at a missing emergency address",
		Long: `Report E911 data gaps across every client, as input to a Kari's Law / RAY BAUM's Act review.
It checks caller-ID and address-ID data only; it does not test 911 dialing or notifications,
and entities with no emergency address set are not flagged.

The emergency caller ID is resolved through the device -> user -> domain fallback, so a user
is only flagged when neither the user nor its domain has one. An emergency-address-id is
flagged when it does not match any address synced for that domain.`,
		Example: strings.Trim(`
  netsapiens-cli fleet e911 --agent
  netsapiens-cli fleet e911 --domain acme.example --select gaps.entity,gaps.id,gaps.gaps`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet e911")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(ctx, cmd, flags, dbPath, e911Report{Gaps: []e911Gap{}, auditCoverage: noMirrorCoverage()})
			if db == nil || err != nil {
				return err
			}
			defer db.Close()
			cov := fleetCov{}
			var sets [4][]fleetRow
			for i, rt := range []string{rtDomains, rtUsers, rtDevices, rtAddresses} {
				if sets[i], err = loadCovered(ctx, db, cov, rt); err != nil {
					return err
				}
				sets[i] = filterDomain(sets[i], domain)
			}
			rep := e911Gaps(sets[0], sets[1], sets[2], sets[3], e911Coverage{domains: cov[rtDomains], users: cov[rtUsers], devices: cov[rtDevices], addresses: cov[rtAddresses]})
			hintCoverage(cmd, db, rtDomains, rtUsers, rtDevices, rtAddresses)
			rep.auditCoverage = fleetAuditCoverage(cmd, flags, db, cov, []string{rtDomains}, []string{rtUsers}, []string{rtDevices}, []string{rtAddresses})
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// keep: --agent/--compact drop keys present on few rows; device rows carry the owning user.
				return printFleetAudit(cmd, flags, rep.auditCoverage, rep, rep.Gaps, "user", "effective_emergency_caller_id", "emergency_address_id")
			}
			printIncomplete(cmd, rep.auditCoverage)
			fmt.Fprintf(cmd.OutOrStdout(), "Checked %d domains, %d users, %d devices against %d emergency addresses.\n", rep.DomainsChecked, rep.UsersChecked, rep.DevicesChecked, rep.AddressesKnown)
			if rep.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Note: "+rep.Note)
			}
			if len(rep.Gaps) == 0 {
				if rep.Complete {
					fmt.Fprintln(cmd.OutOrStdout(), "No E911 gaps found.")
				}
				return nil
			}
			table := make([]map[string]any, 0, len(rep.Gaps))
			for _, g := range rep.Gaps {
				table = append(table, map[string]any{"domain": g.Domain, "entity": g.Entity, "id": g.ID, "name": g.Name, "gaps": strings.Join(g.Gaps, ",")})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only this client domain")
	return cmd
}
