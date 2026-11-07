// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"sort"
	"strconv"
	"strings"
)

// AgentVersionGroup is one Automate agent version with the computers running it.
type AgentVersionGroup struct {
	Version string         `json:"version"`
	Count   int            `json:"count"`
	Newest  bool           `json:"newest"`
	Clients map[string]int `json:"clients"`
	byKey   map[string]int // per client key; labelled into Clients at the end
}

// AgentVersionReport is the fleet's agent-version spread. Newest is the
// highest version any synced computer reports; Behind counts computers on
// any other known version (unknown versions are counted apart).
type AgentVersionReport struct {
	Newest   string              `json:"newest"`
	Total    int                 `json:"total"`
	Behind   int                 `json:"behind"`
	Unknown  int                 `json:"unknown"`
	Versions []AgentVersionGroup `json:"versions"`
}

const unknownAgentVersion = "(unknown)"

// AgentVersions groups computers by RemoteAgentVersion, newest version first.
// With behindOnly, the newest version's group is left out of Versions (the
// totals still cover the whole fleet).
func AgentVersions(computers, clients []map[string]any, behindOnly bool) AgentVersionReport {
	clientIdx := clientNameIndex(clients)
	groups := map[string]*AgentVersionGroup{}
	rep := AgentVersionReport{Versions: []AgentVersionGroup{}}
	refs := map[string]clientRef{}
	for _, c := range computers {
		v := strings.TrimSpace(Str(c, "RemoteAgentVersion", "remoteAgentVersion"))
		if v == "" {
			v = unknownAgentVersion
		}
		g := groups[v]
		if g == nil {
			g = &AgentVersionGroup{Version: v, Clients: map[string]int{}, byKey: map[string]int{}}
			groups[v] = g
		}
		g.Count++
		ref := computerClient(c, clientIdx)
		refs[ref.key] = ref
		g.byKey[ref.key]++
		rep.Total++
		if v == unknownAgentVersion {
			rep.Unknown++
		} else if rep.Newest == "" || compareVersions(v, rep.Newest) > 0 {
			rep.Newest = v
		}
	}
	labels := clientLabels(refs, clientIdx)
	for v, g := range groups {
		for key, n := range g.byKey {
			g.Clients[labels[key]] += n
		}
		g.byKey = nil
		if v == rep.Newest {
			g.Newest = true
		} else if v != unknownAgentVersion {
			rep.Behind += g.Count
		}
		if behindOnly && g.Newest {
			continue
		}
		rep.Versions = append(rep.Versions, *g)
	}
	sort.Slice(rep.Versions, func(i, j int) bool {
		a, b := rep.Versions[i].Version, rep.Versions[j].Version
		if (a == unknownAgentVersion) != (b == unknownAgentVersion) {
			return b == unknownAgentVersion
		}
		if c := compareVersions(a, b); c != 0 {
			return c > 0
		}
		return a < b
	})
	return rep
}

// compareVersions compares dotted versions part by part: numeric parts as
// numbers, anything else as text, a missing part as lower ("2020.11" <
// "2020.11.1"). Returns -1, 0 or 1.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		if i >= len(pa) {
			return -1
		}
		if i >= len(pb) {
			return 1
		}
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case pa[i] != pb[i]:
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
