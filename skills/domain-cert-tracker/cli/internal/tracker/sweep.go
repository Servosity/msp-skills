// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"domain-cert-tracker-pp-cli/internal/netguard"
	"domain-cert-tracker-pp-cli/internal/store"
)

// Event types a sweep raises.
const (
	EventDomainExpiring    = "domain.expiring"
	EventCertExpiring      = "cert.expiring"
	EventCTNewCert         = "ct.new_cert"
	EventDNSChanged        = "dns.changed"
	EventRegistrarUnlocked = "registrar.unlocked"
)

// EventTypes lists every event type, for validation.
var EventTypes = []string{EventDomainExpiring, EventCertExpiring, EventCTNewCert, EventDNSChanged, EventRegistrarUnlocked}

// Check kinds.
const (
	KindRDAP = "rdap"
	KindCT   = "ct"
	KindTLS  = "tls"
	KindDNS  = "dns"
)

// Sources are the live checks a sweep runs. Each is an interface so tests
// can replace the network.
type Sources struct {
	RDAP interface {
		Lookup(ctx context.Context, domain string) (DomainObs, error)
	}
	CT interface {
		Search(ctx context.Context, domain string) ([]CTCert, error)
		// Certificate downloads one certificate by its crt.sh id, so its
		// full SAN list can replace crt.sh's matched names.
		Certificate(ctx context.Context, id int64) ([]byte, error)
	}
	TLS interface {
		Probe(ctx context.Context, host string, port int) (TLSObs, error)
	}
	DNS   DNSLookup
	Guard *netguard.Guard
}

// CTObs is one domain's CT read: the certificates crt.sh returned (with
// each one's full SAN list where the sweep has read the certificate) and
// the CAA set that applies, as of this sweep, to every DNS name on them,
// keyed by the name whose set governs it (a wildcard's parent).
type CTObs struct {
	Certs []CTCert           `json:"certs"`
	CAA   map[string]NameCAA `json:"caa_by_name"`
}

// SweepOptions controls one sweep.
type SweepOptions struct {
	Client      string
	Days        int
	Kinds       map[string]bool
	Concurrency int
	CheckTime   time.Duration
	MaxSubjects int // 0 means no cap (live dogfood caps it)
	// CertDownloads caps the certificates the CT check downloads from crt.sh
	// in this sweep to read their full SAN lists: 0 means
	// DefaultCertDownloads, negative means none.
	CertDownloads int
	Now           func() time.Time
}

// DefaultCertDownloads is the per-sweep cap on certificate downloads.
// crt.sh is rate limited to one request every two seconds, so the cap
// bounds what a domain with thousands of certificates adds to one sweep;
// certificates past it keep crt.sh's partial names until a later sweep.
const DefaultCertDownloads = 50

// Failure is one check that did not complete.
type Failure struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Clients string `json:"clients"`
	Error   string `json:"error"`
	Refused bool   `json:"refused,omitempty"`
}

// EventRef is one event a sweep raised.
type EventRef struct {
	Seq     int64  `json:"seq"`
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Clients string `json:"clients"`
}

// SweepResult summarizes a sweep.
type SweepResult struct {
	SweepID       int64                     `json:"sweep_id"`
	StartedAt     string                    `json:"started_at"`
	FinishedAt    string                    `json:"finished_at"`
	Status        string                    `json:"status"`
	Clients       int                       `json:"clients"`
	Domains       int                       `json:"domains"`
	Hosts         int                       `json:"hosts"`
	Checks        map[string]map[string]int `json:"checks"`
	Events        []EventRef                `json:"events"`
	FetchFailures []Failure                 `json:"fetch_failures"`
	FirstError    string                    `json:"first_error,omitempty"`
	Skipped       int                       `json:"skipped_by_cap,omitempty"`
	CertNames     CertNameStats             `json:"cert_names"`
	Cursor        string                    `json:"cursor"`
}

// CertNameStats counts, over this sweep's CT checks, how many certificates
// carry their full SAN list read from the certificate itself and how many
// still carry only the names crt.sh matched (issuers audit reports those as
// names_partial, never routine).
type CertNameStats struct {
	Complete       int `json:"complete"`        // full SAN list known (downloaded now or in an earlier sweep)
	Downloaded     int `json:"downloaded"`      // downloaded and verified in this sweep
	Partial        int `json:"partial"`         // crt.sh's matched names only
	DownloadFailed int `json:"download_failed"` // download, parse or crt.sh-row check failed (retried next sweep)
	Deferred       int `json:"deferred"`        // not downloaded: the per-sweep cap or the check time ran out
}

type subject struct {
	kind    string
	key     string // domain, or host:port
	host    string
	port    int
	clients []string
}

// ErrUnknownClient means --client named a client that is not inventoried.
var ErrUnknownClient = errors.New("unknown client")

// ErrAllFailed means a sweep had checks to run and every one failed.
var ErrAllFailed = errors.New("every check in the sweep failed")

// RunSweep runs every selected check for the inventory, stores the results,
// diffs them against each subject's previous successful result and records
// events. A failed check is stored as a failure (and listed in
// fetch_failures); it never replaces the last good state, never advances the
// CT cursor and never raises a "changed" event.
func RunSweep(ctx context.Context, st *store.Store, src Sources, opt SweepOptions) (SweepResult, error) {
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	if opt.Days <= 0 {
		opt.Days = 30
	}
	if opt.Concurrency < 1 {
		opt.Concurrency = 3
	}
	if opt.Concurrency > 4 {
		opt.Concurrency = 4
	}
	if opt.CheckTime <= 0 {
		opt.CheckTime = 60 * time.Second
	}
	kinds := opt.Kinds
	if len(kinds) == 0 {
		kinds = map[string]bool{KindRDAP: true, KindCT: true, KindTLS: true, KindDNS: true}
	}
	clients, err := st.ListClients(ctx)
	if err != nil {
		return SweepResult{}, err
	}
	if opt.Client != "" {
		var keep []store.Client
		for _, c := range clients {
			if c.Name == opt.Client {
				keep = append(keep, c)
			}
		}
		if len(keep) == 0 {
			return SweepResult{}, fmt.Errorf("%w %q: run 'clients list'", ErrUnknownClient, opt.Client)
		}
		clients = keep
	}
	subjects := planSubjects(clients, kinds)
	res := SweepResult{Clients: len(clients), Checks: map[string]map[string]int{}, Events: []EventRef{}, FetchFailures: []Failure{}}
	for _, s := range subjects {
		if s.kind == KindTLS {
			res.Hosts++
		} else if s.kind == KindRDAP {
			res.Domains++
		}
	}
	if opt.MaxSubjects > 0 && len(subjects) > opt.MaxSubjects {
		res.Skipped = len(subjects) - opt.MaxSubjects
		subjects = subjects[:opt.MaxSubjects]
	}
	startSeq, err := st.MaxEventSeq(ctx)
	if err != nil {
		return res, err
	}
	sweepID, err := st.BeginSweep(ctx, opt.Client)
	if err != nil {
		return res, err
	}
	res.SweepID = sweepID
	res.StartedAt = now().UTC().Format(time.RFC3339)

	names := newCertNamer(st, src.CT, opt.CertDownloads, now)
	type outcome struct {
		s    subject
		data any
		err  error
		at   time.Time // when this check finished; its observation's time
	}
	results := make([]outcome, len(subjects))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opt.Concurrency)
	for i, s := range subjects {
		wg.Add(1)
		go func(i int, s subject) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = outcome{s: s, err: ctx.Err(), at: now()}
				return
			}
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, opt.CheckTime)
			defer cancel()
			data, err := runCheck(cctx, src, s, names)
			results[i] = outcome{s: s, data: data, err: err, at: now()}
		}(i, s)
	}
	wg.Wait()
	// Certificate names are facts about one certificate, kept whatever any
	// check's outcome: a verified list is final, a failure is retried later.
	if err := st.PutCertNames(ctx, names.records()); err != nil {
		return res, err
	}
	res.CertNames = names.totals()

	for _, o := range results {
		k := res.Checks[o.s.kind]
		if k == nil {
			k = map[string]int{"ok": 0, "failed": 0}
			res.Checks[o.s.kind] = k
		}
		obs := store.Observation{SweepID: sweepID, Kind: o.s.kind, Subject: o.s.key, ObservedAt: o.at.UTC().Format(time.RFC3339Nano)}
		if o.err == nil && o.s.kind == KindDNS {
			if d, ok := o.data.(DNSObs); ok && !d.OK() {
				o.err = fmt.Errorf("DNS lookups failed: %s", joinErrors(d.Errors))
			}
		}
		if o.err != nil {
			k["failed"]++
			obs.Error = o.err.Error()
			if o.data != nil {
				obs.Data, _ = json.Marshal(o.data)
			}
			f := Failure{Kind: o.s.kind, Subject: o.s.key, Clients: strings.Join(o.s.clients, ", "), Error: obs.Error, Refused: errors.Is(o.err, netguard.ErrRefused)}
			res.FetchFailures = append(res.FetchFailures, f)
			if res.FirstError == "" {
				res.FirstError = o.s.kind + " " + o.s.key + ": " + obs.Error
			}
		} else {
			k["ok"]++
			obs.OK = true
			obs.Data, _ = json.Marshal(o.data)
		}
		// The baseline is the check that ran last before this one, from
		// any other sweep: sweeps overlap, so not the highest lower id.
		prev, hadPrev, err := st.LatestObservationBefore(ctx, o.s.kind, o.s.key, o.at, sweepID, true)
		if err != nil {
			return res, err
		}
		if err := st.PutObservation(ctx, obs); err != nil {
			return res, err
		}
		if o.err != nil {
			continue
		}
		evs, err := diffSubject(ctx, st, sweepID, o.s, o.data, prev, hadPrev, opt.Days, now())
		if err != nil {
			return res, err
		}
		res.Events = append(res.Events, evs...)
	}
	endSeq, err := st.MaxEventSeq(ctx)
	if err != nil {
		return res, err
	}
	if err := st.SetSweepEventRange(ctx, sweepID, startSeq, endSeq); err != nil {
		return res, err
	}
	res.Status = "ok"
	switch {
	case len(subjects) > 0 && len(res.FetchFailures) == len(subjects):
		res.Status = "failed"
	case len(res.FetchFailures) > 0:
		res.Status = "partial"
	}
	if err := st.FinishSweep(ctx, sweepID, res.Status); err != nil {
		return res, err
	}
	res.FinishedAt = now().UTC().Format(time.RFC3339)
	res.Cursor = fmt.Sprintf("%d", endSeq)
	if res.Status == "failed" {
		return res, fmt.Errorf("%w (%d checks); first error: %s", ErrAllFailed, len(subjects), res.FirstError)
	}
	return res, nil
}

func joinErrors(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+m[k])
	}
	return strings.Join(parts, "; ")
}

func planSubjects(clients []store.Client, kinds map[string]bool) []subject {
	owners := map[string]map[string]bool{}
	meta := map[string]subject{}
	add := func(s subject, client string) {
		id := s.kind + "|" + s.key
		if owners[id] == nil {
			owners[id] = map[string]bool{}
			meta[id] = s
		}
		owners[id][client] = true
	}
	for _, c := range clients {
		for _, t := range c.Targets {
			switch t.Kind {
			case "domain":
				for _, k := range []string{KindRDAP, KindCT, KindDNS} {
					if kinds[k] {
						add(subject{kind: k, key: t.Value}, c.Name)
					}
				}
			case "host":
				if kinds[KindTLS] {
					add(subject{kind: KindTLS, key: fmt.Sprintf("%s:%d", t.Value, t.Port), host: t.Value, port: t.Port}, c.Name)
				}
			}
		}
	}
	out := make([]subject, 0, len(meta))
	for id, s := range meta {
		for c := range owners[id] {
			s.clients = append(s.clients, c)
		}
		sort.Strings(s.clients)
		out = append(out, s)
	}
	order := map[string]int{KindRDAP: 0, KindDNS: 1, KindTLS: 2, KindCT: 3}
	sort.Slice(out, func(i, j int) bool {
		if out[i].kind != out[j].kind {
			return order[out[i].kind] < order[out[j].kind]
		}
		return out[i].key < out[j].key
	})
	return out
}

func runCheck(ctx context.Context, src Sources, s subject, names *certNamer) (any, error) {
	switch s.kind {
	case KindRDAP:
		if src.RDAP == nil {
			return nil, errors.New("RDAP source not configured")
		}
		return src.RDAP.Lookup(ctx, s.key)
	case KindCT:
		if src.CT == nil {
			return nil, errors.New("CT source not configured")
		}
		certs, err := src.CT.Search(ctx, s.key)
		if err != nil {
			return nil, err
		}
		if certs == nil {
			certs = []CTCert{}
		}
		// crt.sh names only the identities that matched the query; read the
		// rest from the certificates themselves before judging CAA.
		names.complete(ctx, certs)
		// issuers audit judges each certificate name against the CAA set
		// that applies to that name (RFC 8659 section 3), so the sweep
		// records it here and the audit stays a local read.
		return CTObs{Certs: certs, CAA: CAAForNames(ctx, src.DNS, certs)}, nil
	case KindDNS:
		if src.DNS == nil {
			return nil, errors.New("DNS source not configured")
		}
		return CheckDNS(ctx, src.DNS, s.key), nil
	case KindTLS:
		if src.TLS == nil {
			return nil, errors.New("TLS source not configured")
		}
		g := src.Guard
		if g == nil {
			g = netguard.New()
		}
		// A stored port is never a permission: only the ports allowed for
		// this invocation are dialed.
		if err := g.CheckPort(s.port); err != nil {
			return nil, err
		}
		return src.TLS.Probe(ctx, s.host, s.port)
	}
	return nil, fmt.Errorf("unknown check %q", s.kind)
}

func diffSubject(ctx context.Context, st *store.Store, sweepID int64, s subject, data any, prev store.Observation, hadPrev bool, days int, now time.Time) ([]EventRef, error) {
	owners := strings.Join(s.clients, ", ")
	occurred := now.UTC().Format(time.RFC3339)
	var out []EventRef
	emit := func(typ, id, resourceType string, payload any) error {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		seq, err := st.InsertVendorEvent(ctx, store.VendorEvent{EventID: id, Type: typ, Source: "sweep", OccurredAt: occurred,
			ResourceType: resourceType, ResourceID: s.key, Client: owners, Data: raw})
		if err != nil {
			return err
		}
		if seq > 0 {
			out = append(out, EventRef{Seq: seq, Type: typ, Subject: s.key, Clients: owners})
		}
		return nil
	}
	switch s.kind {
	case KindRDAP:
		cur := data.(DomainObs)
		if cur.Expires != "" {
			if t, err := time.Parse(time.RFC3339, cur.Expires); err == nil {
				if left := DaysUntil(now, t); left <= days {
					if err := emit(EventDomainExpiring, fmt.Sprintf("%s:%s:%s", EventDomainExpiring, s.key, cur.Expires), "domain",
						map[string]any{"domain": s.key, "expires": cur.Expires, "days_left": left, "registrar": cur.Registrar}); err != nil {
						return nil, err
					}
				}
			}
		}
		var before DomainObs
		if hadPrev {
			_ = json.Unmarshal(prev.Data, &before)
		}
		// Only a published, lock-free status list is "unlocked", and only a
		// previous answer that was itself known to be unlocked suppresses the
		// event: an unknown previous state is not a baseline.
		prevUnlocked := hadPrev && before.LockKnown() && !before.TransferLocked
		if cur.LockKnown() && !cur.TransferLocked && !prevUnlocked {
			payload := map[string]any{"domain": s.key, "registrar": cur.Registrar, "statuses": cur.Statuses, "first_observation": !hadPrev}
			if hadPrev {
				payload["previous_statuses"] = before.Statuses
			}
			if err := emit(EventRegistrarUnlocked, fmt.Sprintf("%s:%s:%d", EventRegistrarUnlocked, s.key, sweepID), "domain", payload); err != nil {
				return nil, err
			}
		}
	case KindCT:
		certs := data.(CTObs).Certs
		rows := make([]store.CTCertRow, 0, len(certs))
		for _, c := range certs {
			rows = append(rows, store.CTCertRow{Domain: s.key, CrtID: c.ID, IssuerCAID: c.IssuerCAID, IssuerName: c.IssuerName,
				CommonName: c.CommonName, Names: c.Names, NotBefore: c.NotBefore, NotAfter: c.NotAfter, Serial: c.Serial})
		}
		fresh, err := st.AddCTCerts(ctx, sweepID, rows)
		if err != nil {
			return nil, err
		}
		// The first successful CT read for a domain is the baseline: its
		// certificates are history, not news.
		if !hadPrev {
			return out, nil
		}
		for _, r := range fresh {
			if err := emit(EventCTNewCert, fmt.Sprintf("%s:%s:%d", EventCTNewCert, s.key, r.CrtID), "certificate",
				map[string]any{"domain": s.key, "crt_id": r.CrtID, "issuer": r.IssuerName, "names": r.Names,
					"not_before": r.NotBefore, "not_after": r.NotAfter, "serial": r.Serial,
					"crtsh_url": crtShURL(r.CrtID)}); err != nil {
				return nil, err
			}
		}
	case KindTLS:
		cur := data.(TLSObs)
		if cur.DaysLeft <= days {
			if err := emit(EventCertExpiring, fmt.Sprintf("%s:%s:%s", EventCertExpiring, s.key, CertKey(cur.Issuer, cur.Serial, cur.NotBefore, cur.NotAfter)), "host",
				map[string]any{"host": s.key, "serial": cur.Serial, "not_after": cur.NotAfter, "days_left": cur.DaysLeft,
					"expired": cur.Expired, "issuer": cur.Issuer}); err != nil {
				return nil, err
			}
		}
	case KindDNS:
		if !hadPrev {
			return out, nil
		}
		cur := data.(DNSObs)
		var before DNSObs
		if err := json.Unmarshal(prev.Data, &before); err != nil {
			return out, nil
		}
		if changes := DNSChanges(before, cur); len(changes) > 0 {
			if err := emit(EventDNSChanged, fmt.Sprintf("%s:%s:%d", EventDNSChanged, s.key, sweepID), "domain",
				map[string]any{"domain": s.key, "changes": changes}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// DNSChange is one record set that differs between two observations.
type DNSChange struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

// DNSChanges compares the record sets of two DNS observations.
func DNSChanges(before, after DNSObs) map[string]DNSChange {
	out := map[string]DNSChange{}
	cmp := func(field string, b, a any) {
		if !reflect.DeepEqual(b, a) {
			out[field] = DNSChange{Before: b, After: a}
		}
	}
	cmp("ns", before.NS, after.NS)
	cmp("mx", before.MX, after.MX)
	cmp("null_mx", before.NullMX, after.NullMX)
	cmp("spf", before.SPF, after.SPF)
	cmp("dmarc", before.DMARC, after.DMARC)
	cmp("caa", before.CAA, after.CAA)
	cmp("dnssec_signed", before.DNSKEY > 0, after.DNSKEY > 0)
	return out
}
