// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Shared helpers for the hand-written Umbrella commands (verdict, destination-lists
// diff, allowlist audit, fleet health, hygiene, report-card).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/cliutil"
)

// maxListPages bounds every page walk so a misbehaving pager cannot loop.
const maxListPages = 200

type umbrellaDestList struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Access    string `json:"access"`
	IsGlobal  bool   `json:"isGlobal"`
	CreatedAt int64  `json:"createdAt"`
}

type umbrellaDestination struct {
	ID          string `json:"id"`
	Destination string `json:"destination"`
	Type        string `json:"type"`
	Comment     string `json:"comment"`
	CreatedAt   string `json:"createdAt"`
}

// fetchDestinationLists pages GET /policies/v2/destinationlists.
func fetchDestinationLists(ctx context.Context, c *client.Client) ([]umbrellaDestList, error) {
	out := make([]umbrellaDestList, 0)
	err := pageData(ctx, c, "/policies/v2/destinationlists", func(raw json.RawMessage) (int, error) {
		var page []json.RawMessage
		if err := decodeArray(raw, &page); err != nil {
			return 0, fmt.Errorf("parsing destination lists: %w", err)
		}
		for i, item := range page {
			var l umbrellaDestList
			if err := decodeObject(item, &l); err != nil || l.ID <= 0 || l.Access == "" {
				return 0, fmt.Errorf("destination list row %d has no usable id/access", i)
			}
			out = append(out, l)
		}
		return len(page), nil
	})
	return out, err
}

// fetchDestinations pages GET /policies/v2/destinationlists/{id}/destinations.
func fetchDestinations(ctx context.Context, c *client.Client, listID int64) ([]umbrellaDestination, error) {
	out := make([]umbrellaDestination, 0)
	path := "/policies/v2/destinationlists/" + strconv.FormatInt(listID, 10) + "/destinations"
	err := pageData(ctx, c, path, func(raw json.RawMessage) (int, error) {
		var page []json.RawMessage
		if err := decodeArray(raw, &page); err != nil {
			return 0, fmt.Errorf("parsing destinations of list %d: %w", listID, err)
		}
		for i, item := range page {
			var d umbrellaDestination
			if err := decodeObject(item, &d); err != nil || strings.TrimSpace(d.ID) == "" || strings.TrimSpace(d.Destination) == "" {
				return 0, fmt.Errorf("destination row %d of list %d has no usable id/destination", i, listID)
			}
			out = append(out, d)
		}
		return len(page), nil
	})
	return out, err
}

// pageData walks a page/limit collection whose body is {"data": [...]} (policies
// scope) or a bare array (deployments scope), stopping on a short page.
func pageData(ctx context.Context, c *client.Client, path string, add func(json.RawMessage) (int, error)) error {
	const limit = 100
	for page := 1; page <= maxListPages; page++ {
		raw, err := c.Get(ctx, path, map[string]string{"page": strconv.Itoa(page), "limit": strconv.Itoa(limit)})
		if err != nil {
			return err
		}
		n, err := add(unwrapData(raw))
		if err != nil {
			return err
		}
		if n < limit {
			return nil
		}
	}
	return fmt.Errorf("%s: stopped after %d pages of %d; results would be incomplete", path, maxListPages, limit)
}

// decodeArray decodes a JSON array into v. A null or non-array body is an error:
// an absent collection must never read as an empty (clean) one.
func decodeArray(raw json.RawMessage, v any) error {
	if t := strings.TrimSpace(string(raw)); !strings.HasPrefix(t, "[") {
		return fmt.Errorf("expected a JSON array, got %.40q", t)
	}
	return json.Unmarshal(raw, v)
}

// rawID renders a JSON identifier exactly: a string literal is unquoted and a
// number keeps its source digits (no float64 round-trip, so large IDs stay exact).
func rawID(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// decodeObject decodes a JSON object into v, rejecting null and non-object bodies.
func decodeObject(raw json.RawMessage, v any) error {
	if t := strings.TrimSpace(string(raw)); !strings.HasPrefix(t, "{") {
		return fmt.Errorf("expected a JSON object, got %.40q", t)
	}
	return json.Unmarshal(raw, v)
}

// unwrapData returns the "data" member of a {"status","meta","data"} envelope,
// or the body unchanged when it is not one.
func unwrapData(raw json.RawMessage) json.RawMessage {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if trimmed := strings.TrimSpace(string(raw)); strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(raw, &env); err == nil && len(env.Data) > 0 {
			return env.Data
		}
	}
	return raw
}

// normalizeDestination canonicalizes a destination-list entry or file line and
// classifies it as domain, url, or ipv4 (the API's destination types).
func normalizeDestination(s string) (value, kind string, ok bool) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "#"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return "", "", false
	}
	if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil {
		if ip.To4() == nil {
			return "", "", false // IPv6: Umbrella destinations are domain, url, or ipv4 only
		}
		return ip.To4().String(), "ipv4", true
	}
	defer func() {
		if !ok {
			return
		}
		ip := net.ParseIP(hostOf(value))
		switch {
		case ip != nil && ip.To4() == nil:
			// IPv6 in any wrapping (bare, bracketed, with port or scheme): Umbrella
			// destinations are domain, url, or ipv4 only.
			value, kind, ok = "", "", false
		case ip != nil && kind == "domain":
			// A scheme- or port-wrapped IPv4 host is still an IPv4 destination.
			value, kind = ip.To4().String(), "ipv4"
		}
	}()
	lower := strings.ToLower(s)
	// Anything with a scheme, path, query, or fragment is parsed as a URL.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.ContainsAny(s, "/?#") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			// Only a leading scheme counts; "://" inside a query (?next=https://x) does not.
			if !leadingScheme.MatchString(lower) {
				u, err = url.Parse("http://" + s)
			}
			if err != nil || u == nil || u.Host == "" {
				return "", "", false
			}
		}
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.To4() == nil {
			return "", "", false // IPv6 host: not an Umbrella destination type
		} else if ip == nil && !validHostname(strings.TrimRight(strings.ToLower(u.Hostname()), ".")) {
			return "", "", false
		}
		// Every trim is repeated to a fixed point so normalizing a normalized value is a
		// no-op (diff compares the file against the list's read-back entries).
		host := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
		if p := u.Port(); p != "" {
			host += ":" + p
		}
		// The path is kept exactly (a trailing slash can be meaningful); only a bare
		// root "/" collapses, so "example.com/" stays the domain example.com.
		rest := u.EscapedPath()
		if rest == "/" {
			rest = ""
		}
		if u.RawQuery != "" {
			rest += "?" + u.RawQuery
		}
		if rest == "" {
			// A bare scheme+host is a domain entry; the port is not part of a domain.
			return strings.TrimRight(strings.ToLower(u.Hostname()), "."), "domain", true
		}
		return host + rest, "url", true
	}
	if h, _, err := net.SplitHostPort(lower); err == nil {
		lower = h
	}
	lower = strings.TrimRight(strings.TrimPrefix(lower, "*."), ".")
	if !validHostname(lower) {
		return "", "", false
	}
	return lower, "domain", true
}

var leadingScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

// validHostname accepts DNS-name characters only (letters, digits, '-', '_', '.'),
// so stray punctuation never turns into a bogus domain destination.
func validHostname(h string) bool {
	if h == "" || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") || strings.Contains(h, "..") {
		return false
	}
	for _, r := range h {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// domainCovers reports whether a domain entry on a destination list applies to
// domain: an exact match, or entry is a parent (example.com covers a.example.com).
func domainCovers(entry, domain string) bool {
	entry = strings.TrimRight(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(entry)), "*."), ".")
	domain = strings.TrimRight(strings.ToLower(strings.TrimSpace(domain)), ".")
	if entry == "" || domain == "" {
		return false
	}
	return domain == entry || strings.HasSuffix(domain, "."+entry)
}

// destinationMatches reports whether a list entry applies to the queried value:
// domain entries cover subdomains, URL entries match on their host, IPs match exactly.
func destinationMatches(d umbrellaDestination, value, kind string) bool {
	entry, entryKind, ok := normalizeDestination(d.Destination)
	if !ok {
		return false
	}
	switch kind {
	case "ipv4":
		return entryKind == "ipv4" && entry == value
	case "url":
		host := hostOf(value)
		if ip := net.ParseIP(host); ip != nil {
			return (entryKind == "ipv4" && entry == host) || (entryKind == "url" && strings.HasPrefix(value, entry))
		}
		return (entryKind == "domain" && domainCovers(entry, host)) || (entryKind == "url" && strings.HasPrefix(value, entry))
	default:
		return entryKind == "domain" && domainCovers(entry, value)
	}
}

type categorization struct {
	Status             int      `json:"status"` // -1 malicious, 0 undetermined, 1 safe
	SecurityCategories []string `json:"security_categories"`
	ContentCategories  []string `json:"content_categories"`
}

// parseCategorization accepts the documented Investigate shapes: an object keyed
// by domain ({"a.com": {...}}), one flat object, or an array of either. A flat
// object inside an array names its domain by a key other than the known fields.
// A body that is neither shape is an error, never an empty (clean-looking) result.
func parseCategorization(raw json.RawMessage, fallbackDomain string) (map[string]categorization, error) {
	out := map[string]categorization{}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, item := range arr {
			m, err := parseCategorization(item, "")
			if err != nil {
				return nil, err
			}
			for k, v := range m {
				out[k] = v
			}
		}
		return out, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unrecognized categorization response: %w", err)
	}
	known := map[string]bool{"status": true, "security_categories": true, "content_categories": true}
	if _, flat := obj["status"]; flat {
		if !validStatus(obj["status"]) {
			return out, nil // no usable answer: callers report the domain as missing
		}
		var cat categorization
		if err := json.Unmarshal(raw, &cat); err != nil {
			return nil, fmt.Errorf("categorization record: %w", err)
		}
		name := fallbackDomain
		for k := range obj {
			if !known[k] {
				name = k
			}
		}
		if name != "" {
			out[strings.ToLower(name)] = cat
		}
		return out, nil
	}
	for k, v := range obj {
		// A null or status-less record is no answer for that domain: leave it out
		// so callers report it as missing, never as a zero-value "undetermined".
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(v, &fields); err != nil {
			return nil, fmt.Errorf("categorization for %s: %w", k, err)
		}
		if fields == nil || !validStatus(fields["status"]) {
			continue
		}
		var cat categorization
		if err := json.Unmarshal(v, &cat); err != nil {
			return nil, fmt.Errorf("categorization for %s: %w", k, err)
		}
		out[strings.ToLower(k)] = cat
	}
	return out, nil
}

// categorizeDomains runs Investigate categorization for domains: the single
// GET for one domain, else the bulk POST in batches that respect the documented
// limits (1000 domains and 100KB per request). missing lists requested domains
// the API returned nothing for; forbidden reports a 403 (no investigate scope).
func categorizeDomains(ctx context.Context, c *client.Client, domains []string) (cats map[string]categorization, missing []string, forbidden bool, err error) {
	cats = map[string]categorization{}
	if len(domains) == 0 {
		return cats, nil, false, nil
	}
	params := map[string]string{"showLabels": "true"}
	if len(domains) == 1 {
		raw, err := c.Get(ctx, "/investigate/v2/domains/categorization/"+url.PathEscape(domains[0]), params)
		if isForbidden(err) {
			return cats, domains, true, nil
		}
		if err != nil {
			return nil, nil, false, err
		}
		if cats, err = parseCategorization(raw, domains[0]); err != nil {
			return nil, nil, false, err
		}
	} else {
		for _, batch := range domainBatches(domains) {
			raw, _, err := c.PostQueryWithParams(ctx, "/investigate/v2/domains/categorization", params, batch)
			if isForbidden(err) {
				return cats, domains, true, nil
			}
			if err != nil {
				return nil, nil, false, err
			}
			m, err := parseCategorization(raw, "")
			if err != nil {
				return nil, nil, false, err
			}
			for k, v := range m {
				cats[k] = v
			}
		}
	}
	for _, d := range domains {
		if _, ok := cats[strings.ToLower(d)]; !ok {
			missing = append(missing, d)
		}
	}
	return cats, missing, false, nil
}

// domainBatches splits domains into bulk-categorization requests of at most
// 1000 domains and under the documented 100KB payload (JSON array encoding).
func domainBatches(domains []string) [][]string {
	const maxDomains, maxBytes = 1000, 100_000 - 1024 // headroom for brackets and escaping
	var batches [][]string
	var cur []string
	size := 2
	for _, d := range domains {
		item := len(d) + 3 // quotes and comma
		if len(cur) > 0 && (len(cur) == maxDomains || size+item > maxBytes) {
			batches = append(batches, cur)
			cur, size = nil, 2
		}
		cur = append(cur, d)
		size += item
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// hostOf returns the bare host of a normalized destination: no path, query,
// fragment, or port.
func hostOf(value string) string {
	if strings.HasPrefix(value, "[") { // bracketed IP literal, with or without port
		if end := strings.Index(value, "]"); end > 0 {
			return value[1:end]
		}
	}
	if i := strings.IndexAny(value, "/?#"); i >= 0 {
		value = value[:i]
	}
	if h, _, err := net.SplitHostPort(value); err == nil {
		return h
	}
	return value
}

// validStatus accepts only the documented Investigate statuses (-1, 0, 1); a
// missing or null status is no answer, never "undetermined".
func validStatus(raw json.RawMessage) bool {
	var n *int
	if len(raw) == 0 || json.Unmarshal(raw, &n) != nil || n == nil {
		return false
	}
	return *n >= -1 && *n <= 1
}

// hasDomainHost reports whether a destination has a DNS name for Investigate
// (IPv4 destinations and URLs on an IP host do not).
func hasDomainHost(value, kind string) bool {
	if kind == "ipv4" {
		return false
	}
	return net.ParseIP(hostOf(value)) == nil
}

// statusLabel spells out the Investigate domain status (-1, 0, 1).
func statusLabel(status int) string {
	switch status {
	case -1:
		return "malicious"
	case 1:
		return "safe"
	default:
		return "undetermined"
	}
}

// isForbidden reports a 401/403 so a section can degrade instead of failing the
// whole command when the API key lacks that scope.
func isForbidden(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusUnauthorized)
}

// requireCredentials fails fast when no Umbrella credentials are configured. The
// gateway answers unauthenticated calls with 403, which every degrading section
// would otherwise report as a missing scope while the command exits 0.
func requireCredentials(c *client.Client) error {
	if cliutil.IsVerifyEnv() || c == nil || c.Config == nil {
		return nil
	}
	cfg := c.Config
	if cfg.AccessToken != "" || cfg.AuthHeaderVal != "" {
		return nil
	}
	id := firstNonEmpty(cfg.ClientID, cfg.CiscoUmbrellaApiKey, os.Getenv("CISCO_UMBRELLA_API_KEY"))
	secret := firstNonEmpty(cfg.ClientSecret, cfg.CiscoUmbrellaApiSecret, os.Getenv("CISCO_UMBRELLA_API_SECRET"))
	if id != "" && secret != "" {
		return nil
	}
	return authErr(fmt.Errorf("no Umbrella credentials: set CISCO_UMBRELLA_API_KEY and CISCO_UMBRELLA_API_SECRET (check with 'cisco-umbrella-cli doctor')"))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
