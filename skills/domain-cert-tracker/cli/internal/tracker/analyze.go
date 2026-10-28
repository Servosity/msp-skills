// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"domain-cert-tracker-pp-cli/internal/store"
)

// Unverified is a check whose latest result is a failure, or that never ran.
type Unverified struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
	SweepID int64  `json:"sweep_id,omitempty"`
}

// ClientReport is one client's section of the report.
type ClientReport struct {
	Client          string           `json:"client"`
	DomainsExpiring []map[string]any `json:"domains_expiring"`
	CertsExpiring   []map[string]any `json:"certs_expiring_or_invalid"`
	Unlocked        []map[string]any `json:"unlocked"`
	NewCT           []map[string]any `json:"new_ct_certs"`
	DNSDrift        []map[string]any `json:"dns_drift"`
	Unverified      []Unverified     `json:"unverified"`
}

// Report is the per-client estate summary.
type Report struct {
	LastSweep   *store.Sweep   `json:"last_sweep"`
	Days        int            `json:"days"`
	ChangesFrom string         `json:"changes_since"`
	Clients     []ClientReport `json:"clients"`
	Totals      map[string]int `json:"totals"`
}

func filterClients(clients []store.Client, name string) ([]store.Client, error) {
	if name == "" {
		return clients, nil
	}
	for _, c := range clients {
		if c.Name == name {
			return []store.Client{c}, nil
		}
	}
	return nil, fmt.Errorf("%w %q: run 'clients list'", ErrUnknownClient, name)
}

func latest(ctx context.Context, st *store.Store, kind, subj string) (store.Observation, bool, error) {
	return st.LatestObservation(ctx, kind, subj, 0, false)
}

// BuildReport reads the latest observation of every check for each client,
// plus the change events raised since changesSince. A check whose latest
// result failed, or that never ran, is listed under unverified, so a quiet
// section is never mistaken for a clean one.
func BuildReport(ctx context.Context, st *store.Store, clientName string, days int, changesSince time.Time, now time.Time) (Report, error) {
	clients, err := st.ListClients(ctx)
	if err != nil {
		return Report{}, err
	}
	if clients, err = filterClients(clients, clientName); err != nil {
		return Report{}, err
	}
	rep := Report{Days: days, ChangesFrom: changesSince.UTC().Format(time.RFC3339), Clients: []ClientReport{}, Totals: EmptyTotals()}
	if sw, ok, err := st.LatestSweep(ctx); err != nil {
		return rep, err
	} else if ok {
		rep.LastSweep = &sw
	}
	events, _, err := st.VendorEventsSince(ctx, 0, "", 1<<30)
	if err != nil {
		return rep, err
	}
	for _, c := range clients {
		cr := ClientReport{Client: c.Name, DomainsExpiring: []map[string]any{}, CertsExpiring: []map[string]any{}, Unlocked: []map[string]any{},
			NewCT: []map[string]any{}, DNSDrift: []map[string]any{}, Unverified: []Unverified{}}
		for _, t := range c.Targets {
			if t.Kind == "domain" {
				d := t.Value
				for _, kind := range []string{KindRDAP, KindCT, KindDNS} {
					o, ok, err := latest(ctx, st, kind, d)
					if err != nil {
						return rep, err
					}
					if !ok {
						cr.Unverified = append(cr.Unverified, Unverified{Kind: kind, Subject: d, Reason: "never swept"})
						continue
					}
					if !o.OK {
						cr.Unverified = append(cr.Unverified, Unverified{Kind: kind, Subject: d, Reason: o.Error, SweepID: o.SweepID})
						continue
					}
					if kind != KindRDAP {
						continue
					}
					var obs DomainObs
					if err := json.Unmarshal(o.Data, &obs); err != nil {
						cr.Unverified = append(cr.Unverified, Unverified{Kind: kind, Subject: d, Reason: "stored RDAP result unreadable", SweepID: o.SweepID})
						continue
					}
					if obs.Expires == "" {
						cr.Unverified = append(cr.Unverified, Unverified{Kind: kind, Subject: d, Reason: "registry did not publish an expiration date", SweepID: o.SweepID})
					} else if t, err := time.Parse(time.RFC3339, obs.Expires); err == nil {
						if left := DaysUntil(now, t); left <= days {
							cr.DomainsExpiring = append(cr.DomainsExpiring, map[string]any{"domain": d, "expires": obs.Expires, "days_left": left, "registrar": obs.Registrar, "checked_in_sweep": o.SweepID})
						}
					}
					if !obs.LockKnown() {
						cr.Unverified = append(cr.Unverified, Unverified{Kind: kind, Subject: d, Reason: "registry did not publish status values; transfer lock unknown", SweepID: o.SweepID})
					} else if !obs.TransferLocked {
						cr.Unlocked = append(cr.Unlocked, map[string]any{"domain": d, "registrar": obs.Registrar, "statuses": obs.Statuses, "checked_in_sweep": o.SweepID})
					}
				}
			}
			if t.Kind == "host" {
				key := fmt.Sprintf("%s:%d", t.Value, t.Port)
				o, ok, err := latest(ctx, st, KindTLS, key)
				if err != nil {
					return rep, err
				}
				if !ok {
					cr.Unverified = append(cr.Unverified, Unverified{Kind: KindTLS, Subject: key, Reason: "never swept"})
					continue
				}
				if !o.OK {
					cr.Unverified = append(cr.Unverified, Unverified{Kind: KindTLS, Subject: key, Reason: o.Error, SweepID: o.SweepID})
					continue
				}
				var obs TLSObs
				if err := json.Unmarshal(o.Data, &obs); err != nil {
					cr.Unverified = append(cr.Unverified, Unverified{Kind: KindTLS, Subject: key, Reason: "stored TLS result unreadable", SweepID: o.SweepID})
					continue
				}
				nb, err := time.Parse(time.RFC3339, obs.NotAfter)
				if err != nil {
					continue
				}
				if left := DaysUntil(now, nb); left <= days || !obs.Trusted || !obs.HostnameMatch {
					cr.CertsExpiring = append(cr.CertsExpiring, map[string]any{"host": key, "not_after": obs.NotAfter, "days_left": left,
						"expired": now.After(nb), "issuer": obs.Issuer, "trusted": obs.Trusted, "hostname_match": obs.HostnameMatch,
						"verify_error": obs.VerifyError, "checked_in_sweep": o.SweepID})
				}
			}
		}
		owned := map[string]bool{}
		for _, t := range c.Targets {
			if t.Kind == "domain" {
				owned[t.Value] = true
			}
		}
		for _, e := range events {
			if !owned[e.ResourceID] {
				continue
			}
			ts, err := time.Parse(time.RFC3339, e.OccurredAt)
			if err != nil || ts.Before(changesSince) {
				continue
			}
			var payload map[string]any
			if json.Unmarshal(e.Data, &payload) != nil {
				continue
			}
			payload["event_seq"] = e.Seq
			payload["occurred_at"] = e.OccurredAt
			switch e.Type {
			case EventCTNewCert:
				cr.NewCT = append(cr.NewCT, payload)
			case EventDNSChanged:
				cr.DNSDrift = append(cr.DNSDrift, payload)
			}
		}
		rep.Totals["domains_expiring"] += len(cr.DomainsExpiring)
		rep.Totals["certs_expiring_or_invalid"] += len(cr.CertsExpiring)
		rep.Totals["unlocked"] += len(cr.Unlocked)
		rep.Totals["new_ct_certs"] += len(cr.NewCT)
		rep.Totals["dns_drift"] += len(cr.DNSDrift)
		rep.Totals["unverified"] += len(cr.Unverified)
		rep.Clients = append(rep.Clients, cr)
	}
	return rep, nil
}

// EmptyTotals returns every report total at zero, so readers never see a
// missing key on an empty inventory.
func EmptyTotals() map[string]int {
	return map[string]int{"domains_expiring": 0, "certs_expiring_or_invalid": 0, "unlocked": 0, "new_ct_certs": 0, "dns_drift": 0, "unverified": 0}
}

// RenewalFinding is one host's renewal state.
type RenewalFinding struct {
	Client         string         `json:"client"`
	Host           string         `json:"host"`
	Status         string         `json:"status"`
	Detail         string         `json:"detail"`
	ServedSerial   string         `json:"served_serial,omitempty"`
	ServedNotAfter string         `json:"served_not_after,omitempty"`
	DaysLeft       *int           `json:"days_left,omitempty"`
	ServedInCT     *bool          `json:"served_in_ct,omitempty"`
	Successor      map[string]any `json:"successor,omitempty"`
	// OverdueSince and OverdueSuccessor are set on deploy_stalled only: the
	// moment the deploy grace of the earliest-sighted undeployed successor
	// ran out, and that certificate. It can be older than Successor (the
	// newest), because a newer issuance never restarts the clock.
	OverdueSince     string           `json:"overdue_since,omitempty"`
	OverdueSuccessor map[string]any   `json:"overdue_successor,omitempty"`
	OtherCovering    []map[string]any `json:"other_covering_certs,omitempty"`
	RenewalMode      string           `json:"renewal_mode"`
	CTDomain         string           `json:"ct_domain,omitempty"`
}

// Renewal statuses.
const (
	RenewalOK              = "ok"
	RenewalDeployStalled   = "deploy_stalled"
	RenewalIssuanceStalled = "issuance_stalled"
	RenewalServedNotLogged = "served_not_logged"
	RenewalCTUnverified    = "ct_unverified"
	RenewalTLSUnverified   = "tls_unverified"
	deployGrace            = 24 * time.Hour
)

// Renewals joins each host's served certificate to the CT history of the
// inventoried domain it sits under. windowDays is the "should have renewed
// by now" horizon: a served certificate this close to expiry with no
// successor logged is issuance_stalled, as is one past two thirds of its
// lifetime.
func Renewals(ctx context.Context, st *store.Store, clientName string, windowDays int, now time.Time) ([]RenewalFinding, error) {
	clients, err := st.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	if clients, err = filterClients(clients, clientName); err != nil {
		return nil, err
	}
	out := []RenewalFinding{}
	for _, c := range clients {
		for _, t := range c.Targets {
			if t.Kind != "host" {
				continue
			}
			key := fmt.Sprintf("%s:%d", t.Value, t.Port)
			f := RenewalFinding{Client: c.Name, Host: key, RenewalMode: "unknown"}
			o, ok, err := latest(ctx, st, KindTLS, key)
			if err != nil {
				return nil, err
			}
			var served TLSObs
			if !ok || !o.OK || json.Unmarshal(o.Data, &served) != nil {
				f.Status, f.Detail = RenewalTLSUnverified, "no successful TLS probe of this host yet; run sweep"
				if ok && !o.OK {
					f.Detail = "latest TLS probe failed: " + o.Error
				}
				out = append(out, f)
				continue
			}
			f.ServedSerial, f.ServedNotAfter = served.Serial, served.NotAfter
			nb, errB := time.Parse(time.RFC3339, served.NotBefore)
			na, errA := time.Parse(time.RFC3339, served.NotAfter)
			if errB != nil || errA != nil {
				f.Status, f.Detail = RenewalTLSUnverified, "stored TLS result has no validity dates"
				out = append(out, f)
				continue
			}
			left := DaysUntil(now, na)
			f.DaysLeft = &left
			domain := coveringDomain(c, t.Value)
			f.CTDomain = domain
			if domain == "" {
				f.Status, f.Detail = RenewalCTUnverified, "host is not under any of this client's inventoried domains, so its CT history is not tracked"
				out = append(out, f)
				continue
			}
			ct, ok, err := latest(ctx, st, KindCT, domain)
			if err != nil {
				return nil, err
			}
			if !ok || !ct.OK {
				f.Status, f.Detail = RenewalCTUnverified, "latest CT read for "+domain+" did not succeed; renewal state unknown"
				out = append(out, f)
				continue
			}
			rows, err := st.ListCTCerts(ctx, domain)
			if err != nil {
				return nil, err
			}
			var covering []store.CTCertRow
			for _, r := range rows {
				if CoversHost(r.Names, t.Value) {
					covering = append(covering, r)
				}
			}
			// A successor is a newer certificate from the same issuer
			// organization as the served one. A newer certificate from another
			// CA that also covers the name is usually a different deployment
			// (a CDN, a SaaS front end); it is listed, never called a stalled
			// deploy. Whether a CT entry IS the served certificate is decided
			// by SameCertificate alone (whole issuer DN, serial, validity), so
			// whether the served certificate is logged is a question of
			// identity, not names: crt.sh's matched names may omit the very
			// name (a parent wildcard) that covers this host.
			servedRef := CertRef{Issuer: served.Issuer, Serial: served.Serial, NotBefore: served.NotBefore, NotAfter: served.NotAfter}
			_, servedReadable := servedRef.identity()
			logged := false
			// A newer same-CA certificate whose names are crt.sh's matched
			// names only may cover this host through a name crt.sh did not
			// show: it can prove a successor, never rule one out.
			hidden := 0
			for _, r := range rows {
				if SameCertificate(ctRowRef(r), servedRef) {
					logged = true
				}
				if r.NamesSource == store.NamesFromCertificate || CoversHost(r.Names, t.Value) || !sameIssuerOrg(r.IssuerName, served.Issuer) {
					continue
				}
				rb, e1 := time.Parse(time.RFC3339, r.NotBefore)
				ra, e2 := time.Parse(time.RFC3339, r.NotAfter)
				if e1 != nil || e2 != nil || replaces(rb, ra, nb, na, now) {
					hidden++
				}
			}
			var lineage, successors []store.CTCertRow
			extends := false // a successor outlives the served certificate
			for i := range covering {
				r := &covering[i]
				sameCA := sameIssuerOrg(r.IssuerName, served.Issuer)
				if sameCA {
					lineage = append(lineage, *r)
				}
				rb, e1 := time.Parse(time.RFC3339, r.NotBefore)
				ra, e2 := time.Parse(time.RFC3339, r.NotAfter)
				if e1 != nil || e2 != nil || !replaces(rb, ra, nb, na, now) {
					continue
				}
				if !sameCA {
					f.OtherCovering = append(f.OtherCovering, map[string]any{"crt_id": r.CrtID, "issuer": r.IssuerName, "not_before": r.NotBefore,
						"not_after": r.NotAfter, "crtsh_url": crtShURL(r.CrtID)})
					continue
				}
				successors = append(successors, *r)
				extends = extends || ra.After(na)
			}
			f.RenewalMode = renewalMode(lineage, na.Sub(nb))
			if servedReadable {
				f.ServedInCT = &logged
			}
			var v deployVerdict
			if len(successors) > 0 {
				var err error
				if v, err = judgeDeploy(ctx, st, domain, successors, o); err != nil {
					return nil, err
				}
			}
			switch {
			// A shorter-lived successor counts only once overdue: within its
			// grace it is no evidence this host was renewed.
			case extends || v.status == RenewalDeployStalled:
				f.Successor = v.newest
				f.Status, f.Detail = v.status, v.detail
				if v.status == RenewalDeployStalled {
					f.OverdueSince, f.OverdueSuccessor = v.overdueSince, v.overdue
				}
			case hidden > 0:
				f.Status, f.Detail = RenewalCTUnverified, fmt.Sprintf("%d newer certificate(s) from the same CA are in CT for %s with only the names crt.sh matched, "+
					"so whether one renews this host is unknown; a later sweep downloads them from crt.sh", hidden, domain)
			case left <= windowDays || now.After(nb.Add(na.Sub(nb)*2/3)):
				f.Status, f.Detail = RenewalIssuanceStalled, "past the renewal point with no newer certificate for this host in CT"
			case !servedReadable:
				f.Status, f.Detail = RenewalServedNotLogged, "the served certificate's issuer, serial or validity could not be read, so it cannot be matched to a crt.sh entry"
			case !logged:
				f.Status, f.Detail = RenewalServedNotLogged, "the served certificate is not in crt.sh for this domain (private CA, or not yet indexed)"
			default:
				f.Status, f.Detail = RenewalOK, "served certificate is current and logged"
			}
			out = append(out, f)
		}
	}
	rank := map[string]int{RenewalDeployStalled: 0, RenewalIssuanceStalled: 1, RenewalServedNotLogged: 2, RenewalCTUnverified: 3, RenewalTLSUnverified: 4, RenewalOK: 5}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Status] < rank[out[j].Status] })
	return out, nil
}

// deployVerdict is the deploy state of a host that serves an older
// certificate than some same-CA successors in CT.
type deployVerdict struct {
	status, detail string
	newest         map[string]any // the newest successor
	overdue        map[string]any // the successor whose grace ran out first
	overdueSince   string
}

// replaces reports whether a certificate issued after the served one is a
// replacement for it: it expires later, or, being shorter-lived (RFC 8555 section
// 7.4 lets an order ask for its notAfter), is still valid.
func replaces(rb, ra, nb, na, now time.Time) bool {
	return rb.After(nb) && (ra.After(na) || ra.After(now))
}

// judgeDeploy decides deploy_stalled over every same-CA successor of the
// served certificate, never the newest alone. Each successor's deploy grace
// starts at this tool's own first sighting of it in CT, or at its
// not_before when that is later (crt.sh returns no log timestamp, CAs
// backdate, and a certificate cannot be deployed before it is valid). The
// clock is the earliest of those starts, so a newer issuance never restarts
// it and hides an older successor that was never deployed. Any failure
// signal wins: a successor whose grace ran out before the last probe makes
// the host deploy_stalled; otherwise a successor whose age this tool cannot
// measure against a probe (no dated sighting, or first seen after the last
// probe) makes the host unverified; only then is it ok.
func judgeDeploy(ctx context.Context, st *store.Store, domain string, successors []store.CTCertRow, probe store.Observation) (deployVerdict, error) {
	probedAt, perr := time.Parse(time.RFC3339Nano, probe.ObservedAt)
	var v deployVerdict
	var newestStart, overdueStart time.Time
	undated, unprobed := false, false
	for _, r := range successors {
		ref := map[string]any{"crt_id": r.CrtID, "not_before": r.NotBefore, "not_after": r.NotAfter,
			"issuer": r.IssuerName, "crtsh_url": crtShURL(r.CrtID)}
		rb, _ := time.Parse(time.RFC3339, r.NotBefore) // parsed by the caller
		seenAt, known, err := firstSighting(ctx, st, domain, r.FirstSeenSweep)
		if err != nil {
			return v, err
		}
		if known {
			ref["first_seen_in_ct"] = seenAt.UTC().Format(time.RFC3339)
		}
		if v.newest == nil || rb.After(newestStart) {
			v.newest, newestStart = ref, rb
		}
		switch {
		case !known:
			undated = true
			continue
		case perr != nil || seenAt.After(probedAt):
			// Judged by the times the checks ran, never by sweep ids: a
			// probe that ran before the CT check that first sighted the
			// successor, in the same sweep or an overlapping one, did not
			// see it (handfixes.json renewal-probe-order).
			unprobed = true
			continue
		}
		start := seenAt
		if rb.After(start) {
			start = rb
		}
		if probedAt.Sub(start) > deployGrace && (v.overdue == nil || start.Before(overdueStart)) {
			v.overdue, overdueStart = ref, start
		}
	}
	switch {
	case v.overdue != nil:
		v.overdueSince = overdueStart.Add(deployGrace).UTC().Format(time.RFC3339)
		v.status, v.detail = RenewalDeployStalled, "this tool saw a newer certificate from the same CA in CT more than a day before the last probe, and the host still serves the old one"
	case undated:
		v.status, v.detail = RenewalCTUnverified, "a newer certificate from the same CA is in CT, but this tool has no dated record of first seeing it, so how long it has gone undeployed is unknown"
	case unprobed:
		v.status, v.detail = RenewalTLSUnverified, "this tool has no probe of this host since it first saw a newer certificate from the same CA in CT; run sweep to see whether it was deployed"
	default:
		v.status, v.detail = RenewalOK, "a newer certificate appeared in CT within a day of the last probe; the next sweep shows whether it was deployed"
	}
	return v, nil
}

// firstSighting is when the sweep that first stored a CT certificate read
// crt.sh for domain. known is false when that time cannot be established.
func firstSighting(ctx context.Context, st *store.Store, domain string, sweepID int64) (time.Time, bool, error) {
	at, ok, err := st.ObservationTime(ctx, sweepID, KindCT, domain)
	if err != nil || !ok {
		return time.Time{}, false, err
	}
	t, perr := time.Parse(time.RFC3339Nano, at)
	if perr != nil {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func coveringDomain(c store.Client, host string) string {
	best := ""
	for _, t := range c.Targets {
		if t.Kind == "domain" && (host == t.Value || strings.HasSuffix(host, "."+t.Value)) && len(t.Value) > len(best) {
			best = t.Value
		}
	}
	return best
}

// renewalMode infers automation from the served certificate's own lineage
// (same issuer organization), falling back to the served lifetime alone.
// ponytail: lifetime heuristic only (ACME-issued certs are <= 100 days);
// upgrade to interval regularity if a manual short-lived CA shows up.
func renewalMode(certs []store.CTCertRow, served time.Duration) string {
	lifetimes := []float64{served.Hours() / 24}
	for _, r := range certs {
		b, e1 := time.Parse(time.RFC3339, r.NotBefore)
		a, e2 := time.Parse(time.RFC3339, r.NotAfter)
		if e1 == nil && e2 == nil {
			lifetimes = append(lifetimes, a.Sub(b).Hours()/24)
		}
	}
	if served <= 0 {
		return "unknown"
	}
	sort.Float64s(lifetimes)
	if lifetimes[len(lifetimes)/2] <= 100 {
		return "automated"
	}
	return "manual"
}

// caaIdentifiers maps an issuer organization (substring of the issuer O=
// field, lowercase) to the CAA issuer domains its CA accepts. Unlisted
// issuers are reported as unmapped, never as violations.
var caaIdentifiers = []struct {
	match string
	ids   []string
}{
	{"let's encrypt", []string{"letsencrypt.org"}},
	{"digicert", []string{"digicert.com", "symantec.com", "geotrust.com", "thawte.com", "rapidssl.com"}},
	{"cloudflare", []string{"digicert.com"}},
	{"zerossl", []string{"sectigo.com", "comodoca.com", "zerossl.com"}},
	{"sectigo", []string{"sectigo.com", "comodoca.com", "comodo.com", "usertrust.com", "trust-provider.com"}},
	{"comodo", []string{"sectigo.com", "comodoca.com", "comodo.com", "usertrust.com", "trust-provider.com"}},
	{"godaddy", []string{"godaddy.com", "starfieldtech.com"}},
	{"starfield", []string{"godaddy.com", "starfieldtech.com"}},
	{"amazon", []string{"amazon.com", "amazontrust.com", "awstrust.com", "amazonaws.com"}},
	{"google trust services", []string{"pki.goog"}},
	{"globalsign", []string{"globalsign.com"}},
	{"entrust", []string{"entrust.net", "affirmtrust.com"}},
	{"buypass", []string{"buypass.com", "buypass.no"}},
	{"ssl corporation", []string{"ssl.com"}},
	{"ssl.com", []string{"ssl.com"}},
	{"certainly", []string{"certainly.com"}},
	{"actalis", []string{"actalis.it"}},
	{"harica", []string{"harica.gr"}},
	{"emudhra", []string{"emudhra.com"}},
	{"identrust", []string{"identrust.com"}},
}

func issuerOrg(issuerName string) string { return dnAttr(issuerName, "O") }

// ctRowRef is a stored crt.sh certificate's identity fields.
func ctRowRef(r store.CTCertRow) CertRef {
	return CertRef{Issuer: r.IssuerName, Serial: r.Serial, NotBefore: r.NotBefore, NotAfter: r.NotAfter}
}

func issuerCAAIDs(issuerName string) []string {
	org := strings.ToLower(issuerOrg(issuerName))
	if org == "" {
		org = strings.ToLower(issuerName)
	}
	for _, m := range caaIdentifiers {
		if strings.Contains(org, m.match) {
			return m.ids
		}
	}
	return nil
}

// criticalUnknownCAA names the first property with the issuer-critical flag
// (RFC 8659 4.1, bit 128) whose tag this audit does not interpret. A CA that
// does not understand it must not issue, so no verdict is affirmative.
func criticalUnknownCAA(caa []CAA) string {
	for _, r := range caa {
		switch r.Tag {
		case "issue", "issuewild", "iodef":
			continue
		}
		if r.Flags&128 != 0 {
			return r.Tag
		}
	}
	return ""
}

// caaAllowed returns the issuer domains a CAA set permits for a wildcard or
// non-wildcard certificate (RFC 8659 4.2, 4.3), and whether any property
// applies. An "issue ;" (empty) value forbids every CA.
func caaAllowed(caa []CAA, wildcard bool) (map[string]bool, bool) {
	pick := func(tag string) (map[string]bool, bool) {
		allowed := map[string]bool{}
		found := false
		for _, r := range caa {
			if r.Tag != tag {
				continue
			}
			found = true
			v := strings.TrimSpace(r.Value)
			if i := strings.IndexByte(v, ';'); i >= 0 {
				v = v[:i]
			}
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				allowed[v] = true
			}
		}
		return allowed, found
	}
	if wildcard {
		if a, ok := pick("issuewild"); ok {
			return a, true
		}
	}
	return pick("issue")
}

// Issuer audit statuses. Every certificate in the window gets exactly one;
// only IssuerRoutine is not a finding.
const (
	IssuerNotInCAA      = "not_in_caa"     // the CAA for a name on it does not permit the issuer
	IssuerNamesPartial  = "names_partial"  // only crt.sh's matched names are known, not the certificate's SAN list
	IssuerCAAUnverified = "caa_unverified" // the CAA set for a name on it was not read
	IssuerUnmapped      = "unmapped"       // issuer not in the CA-to-CAA table
	IssuerNew           = "new_issuer"     // first certificate from this issuer for the domain
	IssuerRoutine       = "routine"
)

// IssuerFinding is one certificate whose issuer stands out.
type IssuerFinding struct {
	Client      string   `json:"client"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Detail      string   `json:"detail"`
	CrtID       int64    `json:"crt_id"`
	Issuer      string   `json:"issuer"`
	IssuerOrg   string   `json:"issuer_org"`
	Names       []string `json:"names"`
	NamesSource string   `json:"names_source"`
	NotBefore   string   `json:"not_before"`
	CAA         []string `json:"caa_issuers,omitempty"`
	CrtShURL    string   `json:"crtsh_url"`
}

// IssuerAudit is the issuers audit result. Totals counts every certificate
// in the window by status (routine included).
type IssuerAudit struct {
	WindowStart string          `json:"window_start"`
	Findings    []IssuerFinding `json:"findings"`
	Routine     int             `json:"routine"`
	Totals      map[string]int  `json:"totals"`
	Unverified  []Unverified    `json:"unverified"`
}

// EmptyIssuerAudit is an audit with nothing in it, every total at zero.
func EmptyIssuerAudit(since time.Time) IssuerAudit {
	return IssuerAudit{WindowStart: since.UTC().Format(time.RFC3339), Findings: []IssuerFinding{}, Unverified: []Unverified{},
		Totals: map[string]int{IssuerNotInCAA: 0, IssuerNamesPartial: 0, IssuerCAAUnverified: 0, IssuerUnmapped: 0, IssuerNew: 0, IssuerRoutine: 0}}
}

// AuditIssuers looks at every certificate logged for each client domain
// with a not_before inside the window. Each DNS name on a certificate is
// judged against the CAA set that applies to that name (RFC 8659 section
// 3: its own set, else its closest ancestor's; issuewild only for wildcard
// names, section 4.3), as recorded by the latest successful CT sweep. A
// certificate is not_in_caa when the set of any one of its names does not
// permit the issuer. It also separates first-ever issuers from routine
// same-issuer renewals.
//
// A certificate is routine only when caaVerdict.complete holds: its name
// list was read from the certificate itself and CAA was read for every one
// of those names. Otherwise it is names_partial or caa_unverified, and any
// failure signal (a denial on the names that are known) wins over both.
func AuditIssuers(ctx context.Context, st *store.Store, clientName string, since time.Time) (IssuerAudit, error) {
	clients, err := st.ListClients(ctx)
	if err != nil {
		return IssuerAudit{}, err
	}
	if clients, err = filterClients(clients, clientName); err != nil {
		return IssuerAudit{}, err
	}
	res := EmptyIssuerAudit(since)
	for _, c := range clients {
		for _, t := range c.Targets {
			if t.Kind != "domain" {
				continue
			}
			d := t.Value
			ctObs, ok, err := latest(ctx, st, KindCT, d)
			if err != nil {
				return res, err
			}
			if !ok || !ctObs.OK {
				res.Unverified = append(res.Unverified, Unverified{Kind: KindCT, Subject: d, Reason: "no successful CT read; certificates unknown"})
				continue
			}
			var read CTObs
			_ = json.Unmarshal(ctObs.Data, &read) // an unreadable read leaves every name's CAA unknown
			// Why the last sweep left a certificate's names partial (cap,
			// time, failed download). Explanation only: whether a name list is
			// complete comes from the store, never from this.
			whyPartial := map[int64]string{}
			for _, c := range read.Certs {
				if c.NamesError != "" {
					whyPartial[c.ID] = c.NamesError
				}
			}
			rows, err := st.ListCTCerts(ctx, d)
			if err != nil {
				return res, err
			}
			// Issuer organizations seen before the window, keyed by
			// issuerOrgKey; an issuer that does not read as a DN is never
			// recorded, so it never makes a later certificate routine.
			seenBefore := map[string]bool{}
			hadHistory := false
			for _, r := range rows {
				if nb, err := time.Parse(time.RFC3339, r.NotBefore); err == nil && nb.Before(since) {
					hadHistory = true
					if k, ok := issuerOrgKey(r.IssuerName); ok {
						seenBefore[k] = true
					}
				}
			}
			unknownCAA := map[string]bool{}
			for _, r := range rows {
				nb, err := time.Parse(time.RFC3339, r.NotBefore)
				if err != nil || nb.Before(since) {
					continue
				}
				org := issuerOrg(r.IssuerName)
				f := IssuerFinding{Client: c.Name, Domain: d, CrtID: r.CrtID, Issuer: r.IssuerName, IssuerOrg: org, Names: r.Names,
					NamesSource: r.NamesSource, NotBefore: r.NotBefore, CrtShURL: crtShURL(r.CrtID)}
				v := judgeCAA(read.CAA, r.Names, issuerCAAIDs(r.IssuerName))
				v.namesFromCert = r.NamesSource == store.NamesFromCertificate
				f.CAA = v.allowed
				for _, n := range v.unknown {
					if !unknownCAA[n] {
						unknownCAA[n] = true
						res.Unverified = append(res.Unverified, Unverified{Kind: KindDNS, Subject: n, SweepID: ctObs.SweepID,
							Reason: "the CAA set for this certificate name could not be checked (" + v.why[n] + "); CAA comparison skipped for it"})
					}
				}
				switch {
				case len(v.denied) > 0:
					f.Status, f.Detail = IssuerNotInCAA, "issuer is not permitted by the current CAA for "+strings.Join(v.denied, ", ")+" (CAA may have changed since issuance)"
				case !v.complete() && !v.namesFromCert:
					why := whyPartial[r.CrtID]
					if why == "" {
						why = r.NamesError
					}
					if why == "" {
						why = "certificate not downloaded from crt.sh yet"
					}
					f.Status, f.Detail = IssuerNamesPartial, "only the names crt.sh matched for this domain are known, not the certificate's full name list ("+why+
						"), so CAA was not checked for every name on it; a later sweep downloads it from crt.sh"
				case !v.complete():
					var whys []string
					for _, n := range v.unknown {
						whys = append(whys, n+": "+v.why[n])
					}
					f.Status, f.Detail = IssuerCAAUnverified, "the CAA set could not be checked for "+strings.Join(whys, "; ")+", so the issuer was not checked against it"
				case v.unmapped:
					f.Status, f.Detail = IssuerUnmapped, "issuer is not in the CA-to-CAA table; compare it with the CAA record by hand"
				case hadHistory && !issuerSeenBefore(seenBefore, r.IssuerName):
					f.Status, f.Detail = IssuerNew, "first certificate from this issuer for the domain"
				}
				if f.Status != "" {
					res.Totals[f.Status]++
					res.Findings = append(res.Findings, f)
					continue
				}
				res.Totals[IssuerRoutine]++
				res.Routine++
			}
		}
	}
	return res, nil
}

// issuerSeenBefore reports whether issuerName's organization key was seen
// before the window; an issuer that does not read as a DN never was.
func issuerSeenBefore(seen map[string]bool, issuerName string) bool {
	k, ok := issuerOrgKey(issuerName)
	return ok && seen[k]
}

type caaVerdict struct {
	denied        []string          // "name (CAA at source)" for names whose set forbids the issuer
	unknown       []string          // names whose set is not known
	why           map[string]string // name -> why unknown
	unmapped      bool              // a set applies but the issuer is not in the table
	allowed       []string          // issuer domains the applicable sets permit
	namesFromCert bool              // the names judged are the certificate's own SAN list
}

// complete is the one rule for "CAA was checked for this certificate": its
// names came from the certificate itself and every one had a CAA set read.
// A certificate that fails it is never routine.
func (v caaVerdict) complete() bool { return v.namesFromCert && len(v.unknown) == 0 }

// judgeCAA evaluates every DNS name on one certificate separately.
func judgeCAA(byName map[string]NameCAA, names []string, ids []string) caaVerdict {
	v := caaVerdict{why: map[string]string{}}
	union := map[string]bool{}
	for _, n := range names {
		base, wild, ok := caaName(n)
		if !ok {
			continue
		}
		nc, have := byName[base]
		if !have || nc.Error != "" {
			v.unknown = append(v.unknown, n)
			v.why[n] = "not looked up"
			if nc.Error != "" {
				v.why[n] = nc.Error
			}
			continue
		}
		if len(nc.CAA) == 0 {
			continue // no CAA anywhere above the name: any CA may issue
		}
		if tag := criticalUnknownCAA(nc.CAA); tag != "" {
			v.unknown = append(v.unknown, n)
			v.why[n] = "CAA at " + nc.Source + " has a critical property " + tag + " this audit does not understand"
			continue
		}
		allowed, applies := caaAllowed(nc.CAA, wild)
		if !applies {
			continue
		}
		for k := range allowed {
			union[k] = true
		}
		if ids == nil {
			v.unmapped = true
			continue
		}
		permitted := false
		for _, id := range ids {
			if allowed[id] {
				permitted = true
			}
		}
		if !permitted {
			v.denied = append(v.denied, fmt.Sprintf("%s (CAA at %s)", n, nc.Source))
		}
	}
	for k := range union {
		v.allowed = append(v.allowed, k)
	}
	sort.Strings(v.allowed)
	return v
}

// UnwatchedHost is a CT name the inventory does not list.
type UnwatchedHost struct {
	Client         string   `json:"client"`
	Name           string   `json:"name"`
	Domain         string   `json:"domain"`
	Wildcard       bool     `json:"wildcard"`
	Certificates   int      `json:"certificates"`
	LatestNotAfter string   `json:"latest_not_after"`
	Issuers        []string `json:"issuers"`
}

// HostsResult is the hosts discover result.
type HostsResult struct {
	Hosts      []UnwatchedHost `json:"hosts"`
	Unverified []Unverified    `json:"unverified"`
}

// DiscoverHosts lists names in stored CT entries for each client's domains
// that the client's inventory does not list as a domain or host.
func DiscoverHosts(ctx context.Context, st *store.Store, clientName string) (HostsResult, error) {
	clients, err := st.ListClients(ctx)
	if err != nil {
		return HostsResult{}, err
	}
	if clients, err = filterClients(clients, clientName); err != nil {
		return HostsResult{}, err
	}
	res := HostsResult{Hosts: []UnwatchedHost{}, Unverified: []Unverified{}}
	for _, c := range clients {
		// Only host targets are TLS-probed; a domain target (RDAP, CT, DNS)
		// that also serves TLS is still unwatched for TLS.
		known := map[string]bool{}
		for _, t := range c.Targets {
			if t.Kind == "host" {
				known[t.Value] = true
			}
		}
		found := map[string]*UnwatchedHost{}
		for _, t := range c.Targets {
			if t.Kind != "domain" {
				continue
			}
			read, ok, err := latest(ctx, st, KindCT, t.Value)
			if err != nil {
				return res, err
			}
			if !ok {
				res.Unverified = append(res.Unverified, Unverified{Kind: KindCT, Subject: t.Value, Reason: "no CT read yet; run sweep"})
				continue
			}
			if !read.OK {
				// The names below, if any, come from earlier reads: keep them
				// and say this read failed.
				res.Unverified = append(res.Unverified, Unverified{Kind: KindCT, Subject: t.Value, Reason: read.Error, SweepID: read.SweepID})
			}
			rows, err := st.ListCTCerts(ctx, t.Value)
			if err != nil {
				return res, err
			}
			for _, r := range rows {
				for _, n := range r.Names {
					wild := strings.HasPrefix(n, "*.")
					name := strings.TrimPrefix(n, "*.")
					if !(name == t.Value || strings.HasSuffix(name, "."+t.Value)) || (known[name] && !wild) {
						continue
					}
					label := n
					h := found[label]
					if h == nil {
						h = &UnwatchedHost{Client: c.Name, Name: label, Domain: t.Value, Wildcard: wild, Issuers: []string{}}
						found[label] = h
					}
					h.Certificates++
					if r.NotAfter > h.LatestNotAfter {
						h.LatestNotAfter = r.NotAfter
					}
					h.Issuers = appendUnique(h.Issuers, issuerOrg(r.IssuerName))
				}
			}
		}
		names := make([]string, 0, len(found))
		for n := range found {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			sort.Strings(found[n].Issuers)
			res.Hosts = append(res.Hosts, *found[n])
		}
	}
	return res, nil
}
