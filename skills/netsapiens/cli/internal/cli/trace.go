// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type traceNumber struct {
	PhoneNumber string `json:"phonenumber"`
	Enabled     *bool  `json:"enabled"` // nil when the record omits the field
	Application string `json:"application,omitempty"`
	DestUser    string `json:"destination_user,omitempty"`
	DestHost    string `json:"destination_host,omitempty"`
	Kind        string `json:"kind"` // did | sms
}

type traceUser struct {
	User         string `json:"user"`
	Name         string `json:"name,omitempty"`
	Email        string `json:"email,omitempty"`
	Scope        string `json:"scope,omitempty"`
	Site         string `json:"site,omitempty"`
	CallerID     string `json:"caller_id,omitempty"`
	EmergencyCID string `json:"emergency_caller_id,omitempty"`
	VoicemailOn  *bool  `json:"voicemail_enabled"` // nil when the record omits the field
}

type traceDevice struct {
	Device     string  `json:"device"`
	State      string  `json:"registration_state,omitempty"`
	UserAgent  string  `json:"user_agent,omitempty"`
	LatencyAvg float64 `json:"latency_avg_seconds,omitempty"`
	MAC        string  `json:"mac,omitempty"`
	Model      string  `json:"model,omitempty"`
}

type traceMatch struct {
	Domain     string        `json:"domain"`
	MatchedOn  string        `json:"matched_on"`
	TargetType string        `json:"target_type"`          // user | callqueue | autoattendant | device | external | none | unverified
	Unverified []string      `json:"unverified,omitempty"` // attachments whose source table did not complete a sync
	Target     string        `json:"target,omitempty"`
	TargetName string        `json:"target_name,omitempty"`
	Numbers    []traceNumber `json:"numbers"`
	User       *traceUser    `json:"user,omitempty"`
	Devices    []traceDevice `json:"devices"`
}

// traceCandidate is one stored number an ambiguous input could mean.
type traceCandidate struct {
	PhoneNumber string `json:"phonenumber"` // E.164
	Domain      string `json:"domain"`
	MatchedOn   string `json:"matched_on"` // did | sms | caller-id
}

type traceResult struct {
	Query   string       `json:"query"`
	Kind    string       `json:"kind"`  // the most specific of kinds
	Kinds   []string     `json:"kinds"` // every namespace searched: number | mac | email | extension
	Matches []traceMatch `json:"matches"`
	// Complete is false when a table this lookup needs did not finish its latest
	// sync, so an empty or partial answer is unverified rather than negative.
	Complete bool     `json:"complete"`
	Missing  []string `json:"unsynced_resources,omitempty"`
	Stale    []string `json:"stale_resources,omitempty"`
	Note     string   `json:"note,omitempty"`
	// Ambiguous lists the stored numbers an input could mean when it is more
	// than one; none of them is reported as a match.
	Ambiguous []traceCandidate `json:"ambiguous_candidates,omitempty"`
}

type traceData struct {
	numbers, sms, users, devices, phones, queues, attendants []fleetRow
	synced                                                   map[string]bool // resource type -> latest sync completed
}

// traceNeeds lists the tables a lookup kind depends on for a negative answer.
// "phones" stands for either NDP table (they overlap).
var traceNeeds = map[string][]string{
	"number":    {rtPhonenumbers, rtSmsnumbers, rtUsers},
	"email":     {rtUsers},
	"extension": {rtUsers},
	"mac":       {rtDevices, "phones"},
}

// traceKinds lists every namespace q is valid in, most specific first. One
// identifier can be several: 000011223344 is a documented all-digit MAC and a
// 12-digit number, and any non-email token can be an extension. Each is searched.
func traceKinds(q string) []string {
	if strings.Contains(q, "@") {
		return []string{"email"}
	}
	var kinds []string
	if isMAC(q) {
		kinds = append(kinds, "mac")
	}
	// netsapiens hand-fix (netsapiens-trace-number-needs-phone-syntax): identifier-disambiguation;
	// only phone syntax is a number, so a MAC's digits never match a phone number.
	if len(normDigits(q)) >= 7 && strings.Trim(strings.TrimPrefix(strings.ToLower(q), "tel:"), "0123456789+-(). ") == "" {
		kinds = append(kinds, "number")
	}
	if !isMAC(q) && len(normMAC(q)) == 12 && len(strings.Trim(strings.ToLower(q), "0123456789abcdef:-.")) == 0 {
		kinds = append(kinds, "mac") // all digits: a number first, a MAC too
	}
	return append(kinds, "extension")
}

func traceKind(q string) string { return traceKinds(q)[0] }

// traceTables lists the tables the kinds depend on for a negative answer, with
// "phones" expanded to both NDP tables.
func traceTables(kinds []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range kinds {
		for _, rt := range traceNeeds[k] {
			rts := []string{rt}
			if rt == "phones" {
				rts = rtPhones
			}
			for _, t := range rts {
				if !seen[t] {
					seen[t] = true
					out = append(out, t)
				}
			}
		}
	}
	return out
}

// traceIdentifier walks DID -> routing -> target -> devices -> NDP MAC for one
// identifier across every synced tenant. A number's destination is resolved
// by its dial-rule application, so a to-callqueue route to 700 reports queue
// 700, never user 700 and that user's phones.
func traceIdentifier(q, scope string, td traceData) traceResult {
	kinds := traceKinds(q)
	res := traceResult{Query: q, Kind: kinds[0], Kinds: kinds, Matches: []traceMatch{}}
	usersBy, devicesBy := indexBy(td.users, "user"), indexBy(td.devices, "device")
	queuesBy, aaBy := indexBy(td.queues, "callqueue"), indexBy(td.attendants, "user")
	devicesByUser := map[string][]fleetRow{}
	unownedIn := map[string]int{} // devices with no owner field could belong to any user of the domain
	macUnknown := 0               // devices whose record omits the MAC
	for _, d := range td.devices {
		if !has(d, "device-provisioning-mac-address") {
			macUnknown++
		}
		if !has(d, "user") || d.fstr("user") == "" {
			unownedIn[d.domainOf()]++
			continue // never join devices through an absent owner
		}
		k := key(d.domainOf(), d.fstr("user"))
		devicesByUser[k] = append(devicesByUser[k], d)
	}
	ndpByMAC := map[string]fleetRow{}
	for _, p := range td.phones {
		if mac := normMAC(p.fstr("device-provisioning-mac-address")); mac != "" {
			ndpByMAC[mac] = p
		}
	}
	matches := map[string]*traceMatch{}
	order := []string{}
	get := func(dom, typ, target, on string) *traceMatch {
		k := typ + "\x00" + key(dom, target)
		if m, ok := matches[k]; ok {
			return m
		}
		m := &traceMatch{Domain: dom, MatchedOn: on, TargetType: typ, Target: target, Numbers: []traceNumber{}} // Devices stays nil (null) until known
		matches[k], order = m, append(order, k)
		return m
	}
	inScope := func(dom string) bool { return scope == "" || dom == scope } // domains are case-sensitive
	addNumber := func(n fleetRow, kind string) {
		dom := n.domainOf()
		if !inScope(dom) {
			return
		}
		app := n.fstr("dial-rule-application", "application")
		dest := n.fstr("dial-rule-translation-destination-user", "dest")
		host := n.fstr("dial-rule-translation-destination-host")
		owner := dom
		if host != "" && !strings.ContainsAny(host, "*<") {
			owner = host
		}
		destField, hostField, appField := "dial-rule-translation-destination-user", "dial-rule-translation-destination-host", "dial-rule-application"
		if kind == "sms" {
			destField, appField = "dest", "application"
		}
		typ := "user"
		kindOf := didDestKind
		if kind == "sms" {
			kindOf = smsDestKind
		}
		switch kindOf(app) {
		case "callqueue":
			typ = "callqueue"
		case "device":
			typ = "device"
		case "external":
			typ, owner = "external", dom
		case "parked", "none":
			typ, owner = "none", dom
		case "user":
			if aaBy[key(owner, dest)] != nil {
				typ = "autoattendant"
			}
		}
		if !has(n, appField) {
			typ, owner = "unverified", dom // routing unknown: attach nothing
		}
		if typ == "user" || typ == "callqueue" || typ == "device" || typ == "autoattendant" {
			if !has(n, destField) || (kind != "sms" && !has(n, hostField)) || owner == "" {
				typ, owner = "unverified", dom
			}
		}
		m := get(owner, typ, dest, kind)
		m.Numbers = append(m.Numbers, traceNumber{
			PhoneNumber: n.fstr("phonenumber", "number"), Enabled: optYes(n, "enabled"),
			Application: app, DestUser: dest, DestHost: host, Kind: kind,
		})
	}
	addUserDevice := func(m *traceMatch, d fleetRow) {
		lat, _ := d.fnum("device-sip-latency-seconds-average")
		mac := normMAC(d.fstr("device-provisioning-mac-address"))
		model := d.fstr("device-models-model")
		if p, ok := ndpByMAC[mac]; ok && model == "" {
			model = p.fstr("device-models-brand-and-model")
		}
		m.Devices = append(m.Devices, traceDevice{
			Device: d.fstr("device"), State: d.fstr("device-sip-registration-state"),
			UserAgent: d.fstr("device-sip-registration-user-agent"), LatencyAvg: lat, MAC: mac, Model: model,
		})
	}

	macHits := 0
	for _, kind := range kinds {
		switch kind {
		case "number":
			// netsapiens hand-fix (netsapiens-trace-number-needs-phone-syntax): an
			// input that could be more than one stored number picks none.
			want := phoneCandidates(q)
			var hits []traceCandidate
			var apply []func()
			for _, n := range td.numbers {
				if id := phoneIdentity(n.fstr("phonenumber")); want[id] && inScope(n.domainOf()) {
					n := n
					hits, apply = append(hits, traceCandidate{id, n.domainOf(), "did"}), append(apply, func() { addNumber(n, "did") })
				}
			}
			for _, n := range td.sms {
				if id := phoneIdentity(n.fstr("number")); want[id] && inScope(n.domainOf()) {
					n := n
					hits, apply = append(hits, traceCandidate{id, n.domainOf(), "sms"}), append(apply, func() { addNumber(n, "sms") })
				}
			}
			for _, u := range td.users { // the number may also be a user's caller ID or a long extension
				if id := phoneIdentity(u.fstr("caller-id-number")); want[id] && inScope(u.domainOf()) {
					u := u
					hits, apply = append(hits, traceCandidate{id, u.domainOf(), "caller-id"}), append(apply, func() { get(u.domainOf(), "user", u.fstr("user"), "caller-id") })
				}
			}
			ids := map[string]bool{}
			for _, h := range hits {
				ids[h.PhoneNumber] = true
			}
			if len(ids) > 1 {
				res.Ambiguous = hits
				break
			}
			for _, f := range apply {
				f()
			}
		case "email":
			for _, u := range td.users {
				if inScope(u.domainOf()) && (emailListHas(u.fstr("email"), q) || u.fstr("login-username") == q) {
					get(u.domainOf(), "user", u.fstr("user"), "email")
				}
			}
		case "mac":
			want := normMAC(q)
			for _, d := range td.devices {
				if inScope(d.domainOf()) && normMAC(d.fstr("device-provisioning-mac-address")) == want {
					macHits++
					if has(d, "user") && d.fstr("user") != "" {
						get(d.domainOf(), "user", d.fstr("user"), "mac")
					} else { // owner unknown: report exactly this device, owner unverified
						m := get(d.domainOf(), "device", d.fstr("device"), "mac")
						m.Unverified = append(m.Unverified, "owner")
					}
				}
			}
			if p, ok := ndpByMAC[want]; ok && macHits == 0 && inScope(p.domainOf()) {
				on, typ := "mac-unassigned", "none"
				if !td.synced[rtDevices] || macUnknown > 0 { // an incomplete sync or a device without a MAC field may own it
					on, typ = "mac-provisioned", "unverified"
				}
				m := get(p.domainOf(), typ, "", on)
				m.Devices = append(m.Devices, traceDevice{MAC: want, Model: p.fstr("device-models-brand-and-model")})
			}
		default: // extension: ambiguous across tenants, every domain's match is returned
			for _, u := range td.users {
				if inScope(u.domainOf()) && u.fstr("user") == q {
					get(u.domainOf(), "user", u.fstr("user"), "extension")
				}
			}
		}
	}

	need := func(m *traceMatch, rt, what string) bool {
		if td.synced[rt] {
			return true
		}
		m.Unverified = append(m.Unverified, what)
		return false
	}
	attachDevices := func(m *traceMatch, rows []fleetRow) {
		if !need(m, rtDevices, "devices") {
			return // nil: unknown, not "no phones"
		}
		if m.TargetType == "user" && unownedIn[m.Domain] > 0 {
			m.Unverified = append(m.Unverified, "devices:unowned-devices-in-domain") // the list may be incomplete
		}
		m.Devices = []traceDevice{}
		for _, d := range rows {
			addUserDevice(m, d)
		}
	}
	for _, k := range order {
		m := matches[k]
		tk := key(m.Domain, m.Target)
		switch m.TargetType {
		case "user":
			if need(m, rtUsers, "user") {
				if u := usersBy[tk]; u != nil {
					m.TargetName = fullName(u)
					m.User = &traceUser{
						User: u.fstr("user"), Name: fullName(u), Email: u.fstr("email"), Scope: u.fstr("user-scope"),
						Site: u.fstr("site"), CallerID: u.fstr("caller-id-number"), EmergencyCID: u.fstr("caller-id-number-emergency"),
						VoicemailOn: optYes(u, "voicemail-enabled"),
					}
				}
			}
			if m.MatchedOn == "did" || m.MatchedOn == "sms" {
				need(m, rtAutoattendants, "autoattendant-check") // the target could be an auto attendant
			}
			attachDevices(m, devicesByUser[tk])
		case "device":
			var rows []fleetRow
			if d := devicesBy[tk]; d != nil {
				rows = []fleetRow{d}
			}
			attachDevices(m, rows)
		case "callqueue":
			if need(m, rtCallqueues, "callqueue") {
				if qr := queuesBy[tk]; qr != nil {
					m.TargetName = qr.fstr("description")
				}
			}
		case "autoattendant":
			if aa := aaBy[tk]; aa != nil {
				m.TargetName = aa.fstr("attendant-name")
			}
		}
		res.Matches = append(res.Matches, *m)
	}
	res.Complete = true
	for _, m := range res.Matches {
		if len(m.Unverified) > 0 || m.TargetType == "unverified" {
			res.Complete = false
		}
	}
	seenNeed := map[string]bool{}
	for _, k := range kinds {
		for _, rt := range traceNeeds[k] {
			if seenNeed[rt] {
				continue
			}
			seenNeed[rt] = true
			if !td.synced[rt] {
				res.Complete = false
				res.Missing = append(res.Missing, rt)
			}
		}
	}
	if len(res.Ambiguous) > 0 {
		res.Complete = false
		var ids []string
		seenID := map[string]bool{}
		for _, c := range res.Ambiguous {
			if !seenID[c.PhoneNumber] {
				seenID[c.PhoneNumber] = true
				ids = append(ids, c.PhoneNumber)
			}
		}
		res.Note = "ambiguous: " + q + " could be " + strings.Join(ids, " or ") + "; none is reported. Repeat with the full number and its country code (for example " + ids[0] + ")"
	} else if len(res.Matches) == 0 && len(res.Missing) > 0 {
		res.Note = "no match, but unverified: " + strings.Join(res.Missing, ", ") + " not covered by a completed, unfiltered sync"
	} else if len(res.Matches) == 0 {
		res.Note = "no match in the synced mirror"
	} else if res.Kind == "extension" && len(res.Matches) > 1 && scope == "" {
		res.Note = "extension exists in several domains; pass --domain to pick one"
	}
	return res
}

func newNovelTraceCmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain string
	cmd := &cobra.Command{
		Use:   "trace [number|extension|mac|email]",
		Short: "Resolve one number, extension, MAC or email to its client, routing, user and phones",
		Long: `Use this command to resolve one exact number, extension, MAC, or email to its tenant, routing, user, and phones.
Do NOT use this command for fuzzy free-text lookup by name or description; use 'search' instead.
Do NOT use it for fleet-wide audits; use 'fleet dids' or 'fleet offline' instead.

Numbers match regardless of formatting (+1, dashes, spaces); MACs match with or without separators.`,
		Example: strings.Trim(`
  netsapiens-cli trace 6025550142 --agent
  netsapiens-cli trace 204 --domain acme.example
  netsapiens-cli trace 00:15:65:aa:bb:cc --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "query=6025550142"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trace")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("trace needs exactly one number, extension, MAC or email"))
			}
			q := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetStore(ctx, cmd, flags, dbPath, traceResult{Query: q, Kind: traceKind(q), Kinds: traceKinds(q), Matches: []traceMatch{}, Missing: []string{"local mirror"}})
			if db == nil || err != nil {
				return err
			}
			defer db.Close()
			// Only completed syncs are searched; the rest make a negative answer unverified.
			var td traceData
			cov := fleetCov{}
			for _, p := range []struct {
				dst *[]fleetRow
				rts []string
			}{{&td.numbers, []string{rtPhonenumbers}}, {&td.sms, []string{rtSmsnumbers}}, {&td.users, []string{rtUsers}}, {&td.devices, []string{rtDevices}}, {&td.phones, rtPhones},
				{&td.queues, []string{rtCallqueues}}, {&td.attendants, []string{rtAutoattendants}}} {
				if *p.dst, err = loadCovered(ctx, db, cov, p.rts...); err != nil {
					return err
				}
			}
			td.synced = map[string]bool{"phones": cov.anyCovered(rtPhones...)}
			for _, rt := range []string{rtPhonenumbers, rtSmsnumbers, rtUsers, rtDevices, rtCallqueues, rtAutoattendants} {
				td.synced[rt] = cov[rt]
			}
			res := traceIdentifier(q, domain, td)
			hintCoverage(cmd, db, traceTables(res.Kinds)...)
			if !res.Complete {
				hintCoverage(cmd, db, rtUsers, rtDevices, rtCallqueues, rtAutoattendants)
			}
			// Every table the answer rests on is judged for age: the ones a negative
			// needs, and the attachment tables once something matched.
			used := traceTables(res.Kinds)
			if len(res.Matches) > 0 {
				used = append(used, rtDevices, rtCallqueues, rtAutoattendants)
				// netsapiens hand-fix (netsapiens-audits-judge-every-dependency): every table
				// a match can attach from (users, NDP phones, ...), not a fixed list.
				used = append(used, cov.tables()...)
			}
			if res.Stale = fleetStale(cmd, flags, db, used...); len(res.Stale) > 0 {
				res.Complete = false
				if len(res.Matches) == 0 && len(res.Missing) == 0 {
					res.Note = "no match, but unverified: stale " + strings.Join(res.Stale, ", ")
				}
			}
			ac := auditCoverage{Complete: res.Complete, Missing: res.Missing, Stale: res.Stale}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// keep: --agent/--compact drop keys present on few matches when an extension is ambiguous.
				return printFleetAudit(cmd, flags, ac, res, res.Matches, "user", "target", "target_name", "unverified", "ambiguous_candidates")
			}
			if len(res.Stale) > 0 || len(res.Missing) > 0 {
				printIncomplete(cmd, ac)
			}
			if len(res.Matches) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No match for %q. %s\n", q, res.Note)
				return nil
			}
			for _, m := range res.Matches {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (matched on %s) -> %s %s %s\n", m.Domain, m.MatchedOn, m.TargetType, m.Target, m.TargetName)
				for _, n := range m.Numbers {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %s -> %s %s@%s\n", strings.ToUpper(n.Kind), n.PhoneNumber, n.Application, n.DestUser, n.DestHost)
				}
				if m.User != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  user %s %s <%s> scope=%s site=%s e911=%s\n", m.User.User, m.User.Name, m.User.Email, m.User.Scope, m.User.Site, m.User.EmergencyCID)
				}
				for _, d := range m.Devices {
					fmt.Fprintf(cmd.OutOrStdout(), "  device %s %s mac=%s %s\n", d.Device, d.State, d.MAC, d.Model)
				}
			}
			if res.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Note: "+res.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only match within this client domain (extensions repeat across domains)")
	return cmd
}

// smsDestKind maps the SmsNumber application enum (user, callqueue, available).
func smsDestKind(app string) string {
	switch app {
	case "callqueue":
		return "callqueue"
	case "available":
		return "parked"
	default:
		return "user"
	}
}

// emailListHas matches one address against a NetSapiens email field, which may
// hold a semicolon- or comma-separated list; addresses compare case-insensitively.
func emailListHas(field, q string) bool {
	for _, e := range strings.FieldsFunc(field, func(r rune) bool { return r == ';' || r == ',' }) {
		if strings.EqualFold(strings.TrimSpace(e), strings.TrimSpace(q)) {
			return true
		}
	}
	return false
}
