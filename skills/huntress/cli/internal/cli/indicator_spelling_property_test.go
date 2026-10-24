// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"huntress-pp-cli/internal/cliutil/testenv"
)

// One address and every spelling of it that blast-radius accepts, on the
// --indicator side and in stored text alike: brackets, a port, an IPv6 zone,
// the IPv4-mapped prefix (dotted and hex), a trailing dot, a label such as
// ip:, host: or mac:, and letter case.
type addressSpellings struct {
	name      string
	spellings []string
	// partials are parts of the address, never the address itself: as an
	// indicator each may match a row as a fragment (or a parent domain as a
	// subdomain), never as exact.
	partials []string
}

var indicatorAddressCases = []addressSpellings{
	{
		name: "ipv4 203.0.113.5",
		spellings: []string{
			"203.0.113.5", "203.0.113.5:443", "[203.0.113.5]", "[203.0.113.5]:443",
			"::ffff:203.0.113.5", "[::ffff:203.0.113.5]:443", "::ffff:cb00:7105",
			"[::FFFF:CB00:7105]:8443", "0:0:0:0:0:ffff:cb00:7105",
			"ip:203.0.113.5", "IP:203.0.113.5:443", "203.0.113.5.",
		},
		partials: []string{"113.5", "0.113.5", ".113.5", "203.0.113", "113.5:443"},
	},
	{
		name: "ipv6 2001:db8::5",
		spellings: []string{
			"2001:db8::5", "2001:DB8:0:0:0:0:0:5", "[2001:db8::5]", "[2001:db8::5]:443",
			"2001:db8::5%eth0", "[2001:db8::5%eth0]:443", "ip:2001:db8::5",
		},
		partials: []string{"2001:db8", "db8::"},
	},
	{
		name: "mac 00:1a:2b:3c:4d:5e",
		spellings: []string{
			"00:1a:2b:3c:4d:5e", "00-1A-2B-3C-4D-5E", "001a.2b3c.4d5e",
			"mac:00:1a:2b:3c:4d:5e", "MAC-00-1a-2b-3c-4d-5e",
		},
		partials: []string{"3c:4d:5e", "1a:2b:3c:4d", "2b-3c-4d"},
	},
	{
		name: "domain evil.example.com",
		spellings: []string{
			"evil.example.com", "EVIL.Example.COM", "evil.example.com.", "evil.example.com:443",
			"[evil.example.com]:443", "host:evil.example.com", "host:evil.example.com:8443",
		},
		// Not "evil": a single-label hostname is exact against the first
		// label of an FQDN by design (TestBlastRadiusParsesAddressTokens).
		partials: []string{"example.com", "evil.example"},
	},
}

// Every accepted spelling, used as the indicator, matches a row holding any
// other accepted spelling of the same address, in free text and as a
// structured value, and matches nothing holding another address.
func TestIndicatorSpellingsMatchEveryOtherSpelling(t *testing.T) {
	for _, x := range indicatorAddressCases {
		for _, a := range x.spellings {
			m := newIndicatorMatcher(a)
			if m.kind == indicatorText {
				t.Errorf("%s: indicator %q was read as text, not as an address", x.name, a)
				continue
			}
			for _, y := range indicatorAddressCases {
				for _, b := range y.spellings {
					text, value := m.matchText("Beacon to "+b+" blocked"), m.matchValue(b)
					if y.name == x.name {
						if text != matchExact || value != matchExact {
							t.Errorf("%s: indicator %q vs stored %q: text %q value %q, want exact", x.name, a, b, text, value)
						}
					} else if text != "" || value != "" {
						t.Errorf("indicator %q (%s) matched %q (%s): text %q value %q, want no match", a, x.name, b, y.name, text, value)
					}
				}
			}
		}
	}
}

// A part of an address is never the address: as an indicator it is never an
// exact match against any spelling of the whole address, prefixed spellings
// such as ip:203.0.113.5 included.
func TestPartialAddressIsNeverExact(t *testing.T) {
	for _, x := range indicatorAddressCases {
		for _, p := range x.partials {
			m := newIndicatorMatcher(p)
			for _, b := range x.spellings {
				for _, got := range []string{m.matchText("Beacon to " + b + " blocked"), m.matchValue(b)} {
					if got == matchExact {
						t.Errorf("%s: partial indicator %q matched stored %q as exact", x.name, p, b)
					}
				}
			}
		}
	}
	// The review's case: an address after a label is still an address.
	if got := newIndicatorMatcher("113.22").matchText("seen ip:203.0.113.22 today"); got != matchFragment {
		t.Errorf("113.22 in ip:203.0.113.22 = %q, want fragment", got)
	}
}

// The same property through the command, so the SQL prefilter is part of it:
// a store holds every spelling of every address; each spelling, used as the
// indicator, finds every row of its own address as exact and no other row.
func TestBlastRadiusFindsEverySpellingOfTheIndicator(t *testing.T) {
	testenv.Isolate(t)
	var incidents, agents, ports []string
	want := map[string][]string{} // address name -> sorted row keys
	id := 0
	for _, x := range indicatorAddressCases {
		for _, b := range x.spellings {
			id++
			incidents = append(incidents, fmt.Sprintf(`{"id":%d,"organization_id":7,"status":"sent","sent_at":"%s","summary":%q}`, id, hoursAgo(float64(id)), "Beacon to "+b+" blocked"))
			want[x.name] = append(want[x.name], fmt.Sprintf("incident %d", id))
			switch {
			case strings.HasPrefix(x.name, "mac"):
				agents = append(agents, fmt.Sprintf(`{"id":%d,"organization_id":7,"hostname":"ws%d","mac_addresses":[%q]}`, 1000+id, id, b))
				want[x.name] = append(want[x.name], fmt.Sprintf("agent %d", 1000+id))
			case strings.HasPrefix(x.name, "domain"):
				agents = append(agents, fmt.Sprintf(`{"id":%d,"organization_id":7,"hostname":%q}`, 1000+id, b))
				want[x.name] = append(want[x.name], fmt.Sprintf("agent %d", 1000+id))
			default:
				agents = append(agents, fmt.Sprintf(`{"id":%d,"organization_id":7,"hostname":"ws%d","external_ip":%q}`, 1000+id, id, b))
				ports = append(ports, fmt.Sprintf(`{"id":%d,"ip_address":%q,"port":443}`, 2000+id, b))
				want[x.name] = append(want[x.name], fmt.Sprintf("agent %d", 1000+id), fmt.Sprintf("port %d", 2000+id))
			}
		}
	}
	db := seedStore(t, map[string][]string{
		"accounts_incident_reports": incidents,
		"accounts_agents":           agents,
		"accounts_external_ports":   ports,
	})
	for _, x := range indicatorAddressCases {
		exp := append([]string(nil), want[x.name]...)
		sort.Strings(exp)
		for _, a := range x.spellings {
			out, _ := runNovelObject(t, "blast-radius", "--indicator", a, "--db", db)
			var got []string
			for kind, key := range map[string]string{"incident": "incident_reports", "agent": "agents", "port": "external_ports"} {
				rows, _ := out[key].([]any)
				for _, r := range rows {
					row := r.(map[string]any)
					if row["match"] != matchExact {
						t.Errorf("blast-radius %q: %s %v matched as %v, want exact", a, kind, row["id"], row["match"])
					}
					got = append(got, fmt.Sprintf("%s %v", kind, row["id"]))
				}
			}
			sort.Strings(got)
			if fmt.Sprint(got) != fmt.Sprint(exp) {
				t.Errorf("blast-radius %q (%s) = %v, want %v", a, x.name, got, exp)
			}
		}
	}
}
