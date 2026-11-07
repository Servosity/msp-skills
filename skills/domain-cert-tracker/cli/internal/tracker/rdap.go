// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// IANABootstrapURL is the RFC 9224 DNS bootstrap registry.
const IANABootstrapURL = "https://data.iana.org/rdap/dns.json"

// ErrRDAPUnavailable means no https RDAP server is known for the TLD.
var ErrRDAPUnavailable = errors.New("rdap_unavailable")

// DomainObs is what one RDAP domain lookup says about a registration.
type DomainObs struct {
	Domain           string   `json:"domain"`
	Registrar        string   `json:"registrar,omitempty"`
	RegistrarIANAID  string   `json:"registrar_iana_id,omitempty"`
	Registered       string   `json:"registered,omitempty"`
	Expires          string   `json:"expires,omitempty"`
	Statuses         []string `json:"statuses"`
	TransferLocked   bool     `json:"transfer_locked"`
	StatusNotListed  bool     `json:"status_not_listed,omitempty"`
	Nameservers      []string `json:"nameservers"`
	DelegationSigned *bool    `json:"delegation_signed,omitempty"`
	Server           string   `json:"rdap_server,omitempty"`
	ExpiryNotListed  bool     `json:"expiry_not_listed,omitempty"`
}

// LockKnown reports whether the registry published any status value. RFC
// 9083 section 5.3 makes "status" optional; without it the transfer lock is
// unknown, never "unlocked". A stored observation with no statuses (from
// before StatusNotListed existed) is unknown too.
func (o DomainObs) LockKnown() bool { return !o.StatusNotListed && len(o.Statuses) > 0 }

// NormalizeDomain lowercases, strips a trailing dot and converts a U-label
// name to its A-label (punycode) form. It refuses anything that is not a
// plain host name.
func NormalizeDomain(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), ".")
	if s == "" {
		return "", errors.New("empty domain")
	}
	if strings.ContainsAny(s, "/:@?#\\ \t\r\n") {
		return "", fmt.Errorf("%q is not a domain name", s)
	}
	a, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid domain name: %v", s, err)
	}
	a = strings.ToLower(a)
	if !strings.Contains(a, ".") {
		return "", fmt.Errorf("%q has no top-level domain", s)
	}
	return a, nil
}

// RegisteredDomain returns the registrable name (eTLD+1) RDAP answers for.
func RegisteredDomain(domain string) string {
	if r, err := publicsuffix.EffectiveTLDPlusOne(domain); err == nil {
		return r
	}
	return domain
}

// Bootstrap maps a TLD label to its RDAP base URLs (RFC 9224).
type Bootstrap struct {
	Publication string
	services    map[string][]string
}

// ParseBootstrap reads the IANA dns.json registry.
func ParseBootstrap(body []byte) (*Bootstrap, error) {
	var raw struct {
		Publication string       `json:"publication"`
		Services    [][][]string `json:"services"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse RDAP bootstrap: %w", err)
	}
	b := &Bootstrap{Publication: raw.Publication, services: map[string][]string{}}
	for _, svc := range raw.Services {
		if len(svc) < 2 {
			continue
		}
		for _, tld := range svc[0] {
			tld = strings.ToLower(strings.Trim(tld, "."))
			b.services[tld] = append(b.services[tld], svc[1]...)
		}
	}
	if len(b.services) == 0 {
		return nil, errors.New("parse RDAP bootstrap: no services")
	}
	return b, nil
}

// ServerFor returns the https RDAP base URL for domain, matching the longest
// registered label suffix (RFC 9224 section 4). An http-only entry is
// reported as unavailable: this CLI never sends a lookup over plain http.
func (b *Bootstrap) ServerFor(domain string) (string, error) {
	labels := strings.Split(strings.ToLower(domain), ".")
	for i := 0; i < len(labels); i++ {
		suffix := strings.Join(labels[i:], ".")
		urls, ok := b.services[suffix]
		if !ok {
			continue
		}
		for _, u := range urls {
			if strings.HasPrefix(strings.ToLower(u), "https://") {
				if !strings.HasSuffix(u, "/") {
					u += "/"
				}
				return u, nil
			}
		}
		return "", fmt.Errorf("%w: the .%s registry publishes only a non-https RDAP server", ErrRDAPUnavailable, suffix)
	}
	return "", fmt.Errorf("%w: no RDAP server is registered for %s", ErrRDAPUnavailable, domain)
}

// DomainURL builds the lookup URL for domain on server (RFC 9082 3.1.3).
func DomainURL(server, domain string) string {
	return server + "domain/" + url.PathEscape(domain)
}

// rdapStatusKey folds "client transfer prohibited" and the non-conformant EPP
// spelling "clientTransferProhibited" to the same key.
func rdapStatusKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseRDAPDomain reads an RFC 9083 domain object. An RFC 9083 section 6
// error body, or any object that is not a domain, is an error.
func ParseRDAPDomain(body []byte) (DomainObs, error) {
	var raw struct {
		ObjectClassName string       `json:"objectClassName"`
		LDHName         string       `json:"ldhName"`
		ErrorCode       *int         `json:"errorCode"`
		Title           string       `json:"title"`
		Status          []string     `json:"status"`
		Events          []rdapEvent  `json:"events"`
		Entities        []rdapEntity `json:"entities"`
		Nameservers     []struct {
			LDHName string `json:"ldhName"`
		} `json:"nameservers"`
		SecureDNS *struct {
			DelegationSigned *bool `json:"delegationSigned"`
		} `json:"secureDNS"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return DomainObs{}, fmt.Errorf("parse RDAP domain: %w", err)
	}
	if raw.ErrorCode != nil {
		return DomainObs{}, fmt.Errorf("RDAP error %d: %s", *raw.ErrorCode, raw.Title)
	}
	if !strings.EqualFold(raw.ObjectClassName, "domain") {
		return DomainObs{}, fmt.Errorf("parse RDAP domain: objectClassName %q is not domain", raw.ObjectClassName)
	}
	obs := DomainObs{Domain: strings.ToLower(raw.LDHName), Statuses: []string{}, Nameservers: []string{}}
	for _, s := range raw.Status {
		if strings.TrimSpace(s) == "" {
			continue
		}
		obs.Statuses = append(obs.Statuses, strings.ToLower(strings.TrimSpace(s)))
		switch rdapStatusKey(s) {
		case "clienttransferprohibited", "servertransferprohibited", "transferprohibited", "locked": // RFC 9083 10.2.2: locked forbids any change
			obs.TransferLocked = true
		}
	}
	sort.Strings(obs.Statuses)
	obs.StatusNotListed = len(obs.Statuses) == 0
	for _, e := range raw.Events {
		t, ok := parseRDAPTime(e.EventDate)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(e.EventAction)) {
		case "expiration":
			obs.Expires = t
		case "registration":
			obs.Registered = t
		}
	}
	obs.ExpiryNotListed = obs.Expires == ""
	for _, ns := range raw.Nameservers {
		if n := strings.TrimSuffix(strings.ToLower(ns.LDHName), "."); n != "" {
			obs.Nameservers = append(obs.Nameservers, n)
		}
	}
	sort.Strings(obs.Nameservers)
	if raw.SecureDNS != nil {
		obs.DelegationSigned = raw.SecureDNS.DelegationSigned
	}
	for _, ent := range raw.Entities {
		if !hasRole(ent.Roles, "registrar") {
			continue
		}
		obs.Registrar = ent.displayName()
		for _, id := range ent.PublicIDs {
			if strings.EqualFold(id.Type, "IANA Registrar ID") {
				obs.RegistrarIANAID = id.Identifier
			}
		}
		break
	}
	return obs, nil
}

type rdapEvent struct {
	EventAction string `json:"eventAction"`
	EventDate   string `json:"eventDate"`
}

type rdapEntity struct {
	Handle     string            `json:"handle"`
	Roles      []string          `json:"roles"`
	VCardArray []json.RawMessage `json:"vcardArray"`
	PublicIDs  []struct {
		Type       string `json:"type"`
		Identifier string `json:"identifier"`
	} `json:"publicIds"`
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if strings.EqualFold(strings.TrimSpace(r), want) {
			return true
		}
	}
	return false
}

// displayName reads the jCard (RFC 7095) "fn" property, else the handle.
func (e rdapEntity) displayName() string {
	if len(e.VCardArray) == 2 {
		var props [][]json.RawMessage
		if json.Unmarshal(e.VCardArray[1], &props) == nil {
			for _, p := range props {
				if len(p) < 4 {
					continue
				}
				var name, val string
				if json.Unmarshal(p[0], &name) == nil && strings.EqualFold(name, "fn") && json.Unmarshal(p[3], &val) == nil && strings.TrimSpace(val) != "" {
					return strings.TrimSpace(val)
				}
			}
		}
	}
	return e.Handle
}

// parseRDAPTime accepts RFC 3339 with or without fractional seconds and
// returns it normalized to UTC.
func parseRDAPTime(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339), true
		}
	}
	return "", false
}

// RDAP looks domains up with the IANA bootstrap and the registry server.
type RDAP struct {
	Web       *Web
	mu        sync.Mutex // guards bootstrap: sweep looks domains up concurrently
	bootstrap *Bootstrap
}

// LoadBootstrap fetches and parses the IANA registry once per sweep.
func (r *RDAP) LoadBootstrap(ctx context.Context) error {
	body, err := r.Web.Get(ctx, IANABootstrapURL, "application/json")
	if err != nil {
		return err
	}
	b, err := ParseBootstrap(body)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.bootstrap = b
	r.mu.Unlock()
	return nil
}

// SetBootstrap installs an already parsed registry (tests, cached copies).
func (r *RDAP) SetBootstrap(b *Bootstrap) {
	r.mu.Lock()
	r.bootstrap = b
	r.mu.Unlock()
}

// registry returns the bootstrap registry, loading it once; concurrent
// callers wait for the first load instead of each fetching their own.
func (r *RDAP) registry(ctx context.Context) (*Bootstrap, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bootstrap == nil {
		body, err := r.Web.Get(ctx, IANABootstrapURL, "application/json")
		if err != nil {
			return nil, err
		}
		b, err := ParseBootstrap(body)
		if err != nil {
			return nil, err
		}
		r.bootstrap = b
	}
	return r.bootstrap, nil
}

// Lookup returns the registration of domain's registered name.
func (r *RDAP) Lookup(ctx context.Context, domain string) (DomainObs, error) {
	boot, err := r.registry(ctx)
	if err != nil {
		return DomainObs{}, fmt.Errorf("RDAP bootstrap: %w", err)
	}
	reg := RegisteredDomain(domain)
	server, err := boot.ServerFor(reg)
	if err != nil {
		return DomainObs{}, err
	}
	body, err := r.Web.Get(ctx, DomainURL(server, reg), "application/rdap+json, application/json")
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return DomainObs{}, fmt.Errorf("RDAP: %s is not registered or not known to %s", reg, server)
		}
		return DomainObs{}, err
	}
	obs, err := ParseRDAPDomain(body)
	if err != nil {
		return DomainObs{}, err
	}
	if obs.Domain == "" {
		obs.Domain = reg
	}
	obs.Server = server
	return obs, nil
}
