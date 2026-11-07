// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// DNS record types the stdlib resolver cannot ask for.
const (
	TypeCAA    uint16 = 257
	TypeDNSKEY uint16 = 48
)

// CAA is one RFC 8659 record.
type CAA struct {
	Flags uint8  `json:"flags"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// MX is one mail exchanger.
type MX struct {
	Host string `json:"host"`
	Pref uint16 `json:"pref"`
}

// DNSObs is one domain's mail, delegation and certificate-policy records.
type DNSObs struct {
	Domain      string            `json:"domain"`
	NS          []string          `json:"ns"`
	MX          []MX              `json:"mx"`
	NullMX      bool              `json:"null_mx"`
	SPF         []string          `json:"spf"`
	DMARC       []string          `json:"dmarc"`
	DMARCPolicy string            `json:"dmarc_policy,omitempty"`
	CAA         []CAA             `json:"caa"`
	CAASource   string            `json:"caa_source,omitempty"`
	DNSKEY      int               `json:"dnskey_records"`
	Findings    []string          `json:"findings"`
	Errors      map[string]string `json:"errors,omitempty"`
}

// OK reports whether every lookup completed (an empty answer counts).
func (o DNSObs) OK() bool { return len(o.Errors) == 0 }

// DNSLookup is the resolver surface Check needs. Raw returns the RDATA of
// every answer of qtype at name.
type DNSLookup interface {
	NS(ctx context.Context, name string) ([]string, error)
	MX(ctx context.Context, name string) ([]MX, error)
	TXT(ctx context.Context, name string) ([]string, error)
	Raw(ctx context.Context, name string, qtype uint16) ([][]byte, error)
}

// CheckDNS gathers NS, MX, SPF, DMARC, CAA and DNSKEY for domain. A name
// that does not exist or has no records of a type is an empty answer; any
// other failure is recorded in Errors so it is never read as "no records".
func CheckDNS(ctx context.Context, l DNSLookup, domain string) DNSObs {
	o := DNSObs{Domain: domain, NS: []string{}, MX: []MX{}, SPF: []string{}, DMARC: []string{}, CAA: []CAA{}, Findings: []string{}}
	fail := func(what string, err error) {
		if o.Errors == nil {
			o.Errors = map[string]string{}
		}
		o.Errors[what] = err.Error()
	}
	if ns, err := l.NS(ctx, domain); err != nil && !isNotFound(err) {
		fail("ns", err)
	} else {
		for _, n := range ns {
			o.NS = append(o.NS, strings.TrimSuffix(strings.ToLower(n), "."))
		}
		sort.Strings(o.NS)
	}
	if mx, err := l.MX(ctx, domain); err != nil && !isNotFound(err) {
		fail("mx", err)
	} else {
		for _, m := range mx {
			h := strings.TrimSuffix(strings.ToLower(m.Host), ".")
			if h == "" {
				o.NullMX = true // RFC 7505 null MX: the domain accepts no mail
				continue
			}
			o.MX = append(o.MX, MX{Host: h, Pref: m.Pref})
		}
		sort.Slice(o.MX, func(i, j int) bool {
			if o.MX[i].Pref != o.MX[j].Pref {
				return o.MX[i].Pref < o.MX[j].Pref
			}
			return o.MX[i].Host < o.MX[j].Host
		})
	}
	if txt, err := l.TXT(ctx, domain); err != nil && !isNotFound(err) {
		fail("spf", err)
	} else {
		for _, t := range txt {
			t = strings.TrimSpace(t)
			if lt := strings.ToLower(t); lt == "v=spf1" || strings.HasPrefix(lt, "v=spf1 ") {
				o.SPF = append(o.SPF, t)
			}
		}
	}
	if txt, err := l.TXT(ctx, "_dmarc."+domain); err != nil && !isNotFound(err) {
		fail("dmarc", err)
	} else {
		for _, t := range txt {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
				o.DMARC = append(o.DMARC, strings.TrimSpace(t))
			}
		}
		if len(o.DMARC) == 1 {
			o.DMARCPolicy = dmarcTag(o.DMARC[0], "p")
		}
	}
	if set, source, err := RelevantCAA(ctx, l, domain); err != nil {
		fail("caa", err)
	} else if len(set) > 0 {
		o.CAA, o.CAASource = set, source
	}
	if rd, err := l.Raw(ctx, domain, TypeDNSKEY); err != nil && !isNotFound(err) {
		fail("dnskey", err)
	} else {
		o.DNSKEY = len(rd)
	}
	o.Findings = dnsFindings(o)
	return o
}

// CAALookup is the resolver surface CAA evaluation needs.
type CAALookup interface {
	Raw(ctx context.Context, name string, qtype uint16) ([][]byte, error)
}

// NameCAA is the CAA RRset that applies to one certificate name, or why it
// is unknown.
type NameCAA struct {
	CAA    []CAA  `json:"caa"`
	Source string `json:"source,omitempty"`
	Error  string `json:"error,omitempty"`
}

// errCAAQueryCap stops one check from sending unbounded CAA queries.
var errCAAQueryCap = errors.New("not looked up: CAA query limit for this check reached")

// caaResolver evaluates RFC 8659 section 3 for many names, caching the RRset
// found at each queried name so names that share parents share queries.
type caaResolver struct {
	l     CAALookup
	limit int // 0 means no limit
	sent  int
	at    map[string][]CAA
	fails map[string]error
}

func newCAAResolver(l CAALookup, limit int) *caaResolver {
	return &caaResolver{l: l, limit: limit, at: map[string][]CAA{}, fails: map[string]error{}}
}

func (r *caaResolver) query(ctx context.Context, name string) ([]CAA, error) {
	if set, ok := r.at[name]; ok {
		return set, nil
	}
	if err, ok := r.fails[name]; ok {
		return nil, err
	}
	if r.limit > 0 && r.sent >= r.limit {
		return nil, errCAAQueryCap
	}
	r.sent++
	rd, err := r.l.Raw(ctx, name, TypeCAA)
	if err != nil && !isNotFound(err) {
		r.fails[name] = err
		return nil, err
	}
	set := []CAA{}
	for _, d := range rd {
		if c, ok := parseCAA(d); ok {
			set = append(set, c)
		}
	}
	if len(rd) > 0 && len(set) == 0 {
		err := fmt.Errorf("CAA RRset at %s is malformed", name)
		r.fails[name] = err
		return nil, err
	}
	// Sorted (tag, value, flags) so answer order never reads as a change.
	sort.Slice(set, func(i, j int) bool {
		if set[i].Tag != set[j].Tag {
			return set[i].Tag < set[j].Tag
		}
		if set[i].Value != set[j].Value {
			return set[i].Value < set[j].Value
		}
		return set[i].Flags < set[j].Flags
	})
	r.at[name] = set
	return set, nil
}

// relevant returns the RFC 8659 section 3 relevant RRset for name: the CAA
// set at the closest name at or above it, climbing up to but not including
// the root. A wildcard name ("*.x") uses the set for "x". An empty set means
// CAA does not restrict issuance; any failed lookup on the way is an error.
func (r *caaResolver) relevant(ctx context.Context, name string) ([]CAA, string, error) {
	n := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "*."), ".")
	for n != "" {
		set, err := r.query(ctx, n)
		if err != nil {
			return nil, "", fmt.Errorf("CAA lookup for %s: %w", n, err)
		}
		if len(set) > 0 {
			return set, n, nil
		}
		i := strings.IndexByte(n, '.')
		if i < 0 {
			break
		}
		n = n[i+1:]
	}
	return []CAA{}, "", nil
}

// RelevantCAA returns the CAA RRset RFC 8659 section 3 applies to name and
// the name it was found at.
func RelevantCAA(ctx context.Context, l CAALookup, name string) ([]CAA, string, error) {
	return newCAAResolver(l, 0).relevant(ctx, name)
}

// caaName returns the name whose relevant CAA set governs certificate name
// n (the name itself, or a wildcard's parent) and whether n is a wildcard.
// IP addresses and anything that is not a DNS name are not governed by CAA.
func caaName(n string) (string, bool, bool) {
	n = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), ".")
	wild := strings.HasPrefix(n, "*.")
	base := strings.TrimPrefix(n, "*.")
	if base == "" || strings.ContainsAny(base, "@:/ *") || net.ParseIP(base) != nil || !strings.Contains(base, ".") {
		return "", false, false
	}
	return base, wild, true
}

// maxCAAQueriesPerCheck bounds the CAA queries one CT check sends.
const maxCAAQueriesPerCheck = 400

// CAAForNames records the relevant CAA set for every DNS name on certs.
func CAAForNames(ctx context.Context, l CAALookup, certs []CTCert) map[string]NameCAA {
	out := map[string]NameCAA{}
	if l == nil {
		return out
	}
	r := newCAAResolver(l, maxCAAQueriesPerCheck)
	for _, c := range certs {
		for _, n := range c.Names {
			base, _, ok := caaName(n)
			if !ok {
				continue
			}
			if _, done := out[base]; done {
				continue
			}
			set, source, err := r.relevant(ctx, base)
			if err != nil {
				out[base] = NameCAA{CAA: []CAA{}, Error: err.Error()}
				continue
			}
			out[base] = NameCAA{CAA: set, Source: source}
		}
	}
	return out
}

func dnsFindings(o DNSObs) []string {
	f := []string{}
	if _, failed := o.Errors["ns"]; !failed && len(o.NS) == 0 {
		f = append(f, "no NS records")
	}
	if _, failed := o.Errors["spf"]; !failed {
		switch {
		case len(o.SPF) > 1:
			f = append(f, "more than one SPF record (RFC 7208 permerror: receivers treat SPF as failed)")
		case len(o.SPF) == 0 && !o.NullMX:
			f = append(f, "no SPF record")
		}
	}
	if _, failed := o.Errors["dmarc"]; !failed {
		switch {
		case len(o.DMARC) > 1:
			f = append(f, "more than one DMARC record (RFC 7489: receivers ignore DMARC)")
		case len(o.DMARC) == 0:
			f = append(f, "no DMARC record")
		case o.DMARCPolicy == "none":
			f = append(f, "DMARC policy is none (monitoring only)")
		}
	}
	if _, failed := o.Errors["caa"]; !failed && len(o.CAA) == 0 {
		f = append(f, "no CAA record (any CA may issue)")
	}
	if _, failed := o.Errors["dnskey"]; !failed && o.DNSKEY == 0 {
		f = append(f, "zone is not DNSSEC-signed (no DNSKEY)")
	}
	return f
}

func dmarcTag(rec, tag string) string {
	for _, part := range strings.Split(rec, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), tag) {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

// parseCAA reads RFC 8659 section 4.1 RDATA: flags, tag length, tag, value.
func parseCAA(d []byte) (CAA, bool) {
	if len(d) < 2 {
		return CAA{}, false
	}
	tl := int(d[1])
	if tl == 0 || 2+tl > len(d) {
		return CAA{}, false
	}
	return CAA{Flags: d[0], Tag: strings.ToLower(string(d[2 : 2+tl])), Value: string(d[2+tl:])}, true
}

func isNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

// SystemDNS answers through the operating system's configured resolver.
// NS, MX and TXT use Go's resolver; CAA and DNSKEY are sent as raw queries
// to the same resolver address Go's own lookups used, so the argument (a
// domain) never chooses which server is contacted.
type SystemDNS struct {
	once     sync.Once
	mu       sync.Mutex
	server   string
	resolver *net.Resolver
}

func (s *SystemDNS) r() *net.Resolver {
	s.once.Do(func() {
		s.resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			s.mu.Lock()
			if s.server == "" {
				s.server = address
			}
			s.mu.Unlock()
			var d net.Dialer
			return d.DialContext(ctx, network, address)
		}}
	})
	return s.resolver
}

func (s *SystemDNS) NS(ctx context.Context, name string) ([]string, error) {
	ns, err := s.r().LookupNS(ctx, name)
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.Host)
	}
	return out, err
}

func (s *SystemDNS) MX(ctx context.Context, name string) ([]MX, error) {
	mx, err := s.r().LookupMX(ctx, name)
	out := make([]MX, 0, len(mx))
	for _, m := range mx {
		out = append(out, MX{Host: m.Host, Pref: m.Pref})
	}
	return out, err
}

func (s *SystemDNS) TXT(ctx context.Context, name string) ([]string, error) {
	return s.r().LookupTXT(ctx, name)
}

// Raw sends one query for qtype at name to the system resolver.
func (s *SystemDNS) Raw(ctx context.Context, name string, qtype uint16) ([][]byte, error) {
	s.mu.Lock()
	server := s.server
	s.mu.Unlock()
	if server == "" {
		// Any stdlib lookup records the resolver address Go chose.
		_, _ = s.r().LookupNS(ctx, name)
		s.mu.Lock()
		server = s.server
		s.mu.Unlock()
		if server == "" {
			return nil, errors.New("no system DNS resolver is configured")
		}
	}
	return rawQuery(ctx, server, name, qtype)
}

func rawQuery(ctx context.Context, server, name string, qtype uint16) ([][]byte, error) {
	qn, err := dnsmessage.NewName(strings.TrimSuffix(name, ".") + ".")
	if err != nil {
		return nil, err
	}
	var idb [2]byte
	if _, err := rand.Read(idb[:]); err != nil {
		return nil, err
	}
	id := binary.BigEndian.Uint16(idb[:])
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(dnsmessage.Question{Name: qn, Type: dnsmessage.Type(qtype), Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	if err := b.StartAdditionals(); err != nil {
		return nil, err
	}
	var opt dnsmessage.ResourceHeader
	if err := opt.SetEDNS0(1232, dnsmessage.RCodeSuccess, false); err != nil {
		return nil, err
	}
	if err := b.OPTResource(opt, dnsmessage.OPTResource{}); err != nil {
		return nil, err
	}
	q, err := b.Finish()
	if err != nil {
		return nil, err
	}
	resp, err := exchange(ctx, "udp", server, q)
	if err != nil {
		return nil, err
	}
	var p dnsmessage.Parser
	h, err := p.Start(resp)
	if err != nil {
		return nil, err
	}
	if h.Truncated {
		if resp, err = exchange(ctx, "tcp", server, q); err != nil {
			return nil, err
		}
		if h, err = p.Start(resp); err != nil {
			return nil, err
		}
	}
	if h.ID != id {
		return nil, errors.New("DNS answer ID does not match the query")
	}
	switch h.RCode {
	case dnsmessage.RCodeSuccess:
	case dnsmessage.RCodeNameError:
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	default:
		return nil, fmt.Errorf("DNS %s query for %s: %s", typeName(qtype), name, h.RCode)
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, err
	}
	var out [][]byte
	for {
		rh, err := p.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return nil, err
		}
		if uint16(rh.Type) != qtype {
			if err := p.SkipAnswer(); err != nil {
				return nil, err
			}
			continue
		}
		r, err := p.UnknownResource()
		if err != nil {
			return nil, err
		}
		out = append(out, r.Data)
	}
	return out, nil
}

func typeName(t uint16) string {
	switch t {
	case TypeCAA:
		return "CAA"
	case TypeDNSKEY:
		return "DNSKEY"
	}
	return fmt.Sprintf("TYPE%d", t)
}

func exchange(ctx context.Context, network, server string, q []byte) ([]byte, error) {
	// A DNS message cannot exceed 65535 bytes: the TCP framing (RFC 1035
	// section 4.2.2) is a two-byte length prefix, and a UDP payload is smaller.
	// Refuse before dialing so the uint16 length below cannot wrap.
	size := len(q)
	if size > math.MaxUint16 {
		return nil, fmt.Errorf("dns query of %d bytes exceeds the 65535-byte message limit", size)
	}
	qlen := uint16(size)
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, network, server)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	if network == "tcp" {
		msg := make([]byte, 2+len(q))
		binary.BigEndian.PutUint16(msg, qlen)
		copy(msg[2:], q)
		if _, err := conn.Write(msg); err != nil {
			return nil, err
		}
		var lb [2]byte
		if _, err := io.ReadFull(conn, lb[:]); err != nil {
			return nil, err
		}
		buf := make([]byte, binary.BigEndian.Uint16(lb[:]))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return nil, err
		}
		return buf, nil
	}
	if _, err := conn.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
