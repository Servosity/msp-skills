// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: routing-integrity audit over one PBX's call-routing graph.

package fleet

import (
	"sort"
	"strings"
)

// Destination is Pbx.Destination: where a call is sent.
type Destination struct {
	To     string `json:"To"`
	Number string `json:"Number"`
	Name   string `json:"Name"`
}

// Route is Pbx.Route: an optional prompt plus a destination.
type Route struct {
	Route *Destination `json:"Route"`
}

// TimeRoutes are the out-of-office, holiday and break routes shared by
// ring groups, queues and IVRs (embedded, so JSON decoding is unchanged).
type TimeRoutes struct {
	OutOfOfficeRoute *Route `json:"OutOfOfficeRoute"`
	HolidaysRoute    *Route `json:"HolidaysRoute"`
	BreakRoute       *Route `json:"BreakRoute"`
}

// Member is a ring-group member or queue agent.
type Member struct {
	Number string `json:"Number"`
	Name   string `json:"Name"`
}

// RoutingGraph is everything the audit reads from one PBX. Members, Agents,
// and Forwards are OData navigation properties, so the fetch must $expand them.
type RoutingGraph struct {
	Peers []struct {
		Number string `json:"Number"`
		Name   string `json:"Name"`
		Type   string `json:"Type"`
	}
	Users []struct {
		Number    string `json:"Number"`
		FirstName string `json:"FirstName"`
		LastName  string `json:"LastName"`
		Enabled   *bool  `json:"Enabled"`
	}
	RingGroups []struct {
		Number          string       `json:"Number"`
		Name            string       `json:"Name"`
		Members         []Member     `json:"Members"`
		ForwardNoAnswer *Destination `json:"ForwardNoAnswer"`
		TimeRoutes
	}
	Queues []struct {
		Number          string       `json:"Number"`
		Name            string       `json:"Name"`
		Agents          []Member     `json:"Agents"`
		ForwardNoAnswer *Destination `json:"ForwardNoAnswer"`
		TimeRoutes
	}
	Receptionists []struct {
		Number              string `json:"Number"`
		Name                string `json:"Name"`
		TimeoutForwardDN    string `json:"TimeoutForwardDN"`
		TimeoutForwardType  string `json:"TimeoutForwardType"`
		InvalidKeyForwardDN string `json:"InvalidKeyForwardDN"`
		Forwards            []struct {
			Input       string `json:"Input"`
			ForwardType string `json:"ForwardType"`
			ForwardDN   string `json:"ForwardDN"`
		} `json:"Forwards"`
		TimeRoutes
	}
	InboundRules []struct {
		ID                          *int         `json:"Id"`
		RuleName                    string       `json:"RuleName"`
		Data                        string       `json:"Data"`
		OfficeHoursDestination      *Destination `json:"OfficeHoursDestination"`
		OutOfOfficeHoursDestination *Destination `json:"OutOfOfficeHoursDestination"`
		HolidaysDestination         *Destination `json:"HolidaysDestination"`
	}
}

// Finding is one broken routing reference.
type Finding struct {
	PBX        string `json:"pbx"`
	ObjectType string `json:"object_type"`
	Object     string `json:"object"`
	Field      string `json:"field"`
	Target     string `json:"target,omitempty"`
	TargetType string `json:"target_type,omitempty"`
	Problem    string `json:"problem"` // missing | disabled | unknown (state not reported) | empty
	Error      string `json:"error,omitempty"`
}

// internalTypes are destination/forward types that must resolve to a DN on
// this PBX. External numbers, end-call, and repeat-prompt never do.
var internalTypes = map[string]bool{
	"Extension": true, "Queue": true, "RingGroup": true, "IVR": true, "VoiceMail": true,
	"VoiceMailOfDestination": true, "Fax": true, "RoutePoint": true,
}

// Audit returns every routing reference in g that points at a DN that does
// not exist, or at a disabled user, plus ring groups and queues with nobody in them.
func Audit(pbx string, g RoutingGraph) []Finding {
	exists := map[string]bool{}
	for _, p := range g.Peers {
		if n := strings.TrimSpace(p.Number); n != "" {
			exists[n] = true
		}
	}
	disabled, unknown := map[string]bool{}, map[string]bool{}
	for _, u := range g.Users {
		n := strings.TrimSpace(u.Number)
		exists[n] = exists[n] || n != ""
		switch {
		case u.Enabled == nil:
			// A user whose enabled state is not reported is not known to
			// take calls.
			unknown[n] = true
		case !*u.Enabled:
			disabled[n] = true
		}
	}
	mark := func(n string) {
		if n = strings.TrimSpace(n); n != "" {
			exists[n] = true
		}
	}
	for _, r := range g.RingGroups {
		mark(r.Number)
	}
	for _, q := range g.Queues {
		mark(q.Number)
	}
	for _, r := range g.Receptionists {
		mark(r.Number)
	}

	out := []Finding{}
	check := func(objType, obj, field, target, targetType string) {
		target = strings.TrimSpace(target)
		if target == "" || (targetType != "" && !internalTypes[targetType]) {
			return
		}
		switch {
		case !exists[target]:
			out = append(out, Finding{PBX: pbx, ObjectType: objType, Object: obj, Field: field, Target: target, TargetType: targetType, Problem: "missing"})
		case disabled[target]:
			out = append(out, Finding{PBX: pbx, ObjectType: objType, Object: obj, Field: field, Target: target, TargetType: targetType, Problem: "disabled"})
		case unknown[target]:
			out = append(out, Finding{PBX: pbx, ObjectType: objType, Object: obj, Field: field, Target: target, TargetType: targetType, Problem: "unknown"})
		}
	}
	dest := func(objType, obj, field string, d *Destination) {
		if d != nil {
			check(objType, obj, field, d.Number, d.To)
		}
	}
	route := func(objType, obj, field string, r *Route) {
		if r != nil {
			dest(objType, obj, field, r.Route)
		}
	}
	routes := func(objType, obj string, r TimeRoutes) {
		route(objType, obj, "OutOfOfficeRoute", r.OutOfOfficeRoute)
		route(objType, obj, "HolidaysRoute", r.HolidaysRoute)
		route(objType, obj, "BreakRoute", r.BreakRoute)
	}
	// group audits a ring group or queue: its members and every route out.
	group := func(objType, obj, field string, members []Member, fna *Destination, r TimeRoutes) {
		if len(members) == 0 {
			out = append(out, Finding{PBX: pbx, ObjectType: objType, Object: obj, Field: field, Problem: "empty"})
		}
		for _, m := range members {
			check(objType, obj, field, m.Number, "Extension")
		}
		dest(objType, obj, "ForwardNoAnswer", fna)
		routes(objType, obj, r)
	}
	label := func(number, name string) string {
		if name == "" {
			return number
		}
		return number + " " + name
	}

	for _, r := range g.RingGroups {
		group("ring_group", label(r.Number, r.Name), "Members", r.Members, r.ForwardNoAnswer, r.TimeRoutes)
	}
	for _, q := range g.Queues {
		group("queue", label(q.Number, q.Name), "Agents", q.Agents, q.ForwardNoAnswer, q.TimeRoutes)
	}
	for _, r := range g.Receptionists {
		obj := label(r.Number, r.Name)
		for _, f := range r.Forwards {
			check("ivr", obj, "Forwards["+f.Input+"]", f.ForwardDN, ivrType(f.ForwardType))
		}
		check("ivr", obj, "TimeoutForwardDN", r.TimeoutForwardDN, ivrType(r.TimeoutForwardType))
		check("ivr", obj, "InvalidKeyForwardDN", r.InvalidKeyForwardDN, "")
		routes("ivr", obj, r.TimeRoutes)
	}
	for _, ir := range g.InboundRules {
		obj := strings.TrimSpace(ir.Data + " " + ir.RuleName)
		dest("inbound_rule", obj, "OfficeHoursDestination", ir.OfficeHoursDestination)
		dest("inbound_rule", obj, "OutOfOfficeHoursDestination", ir.OutOfOfficeHoursDestination)
		dest("inbound_rule", obj, "HolidaysDestination", ir.HolidaysDestination)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObjectType != out[j].ObjectType {
			return out[i].ObjectType < out[j].ObjectType
		}
		if out[i].Object != out[j].Object {
			return out[i].Object < out[j].Object
		}
		return out[i].Field < out[j].Field
	})
	return out
}

// ivrType maps Pbx.IVRForwardType to a destination type; CallByName,
// RepeatPrompt, EndCall, and CustomInput have no DN to resolve.
func ivrType(t string) string {
	switch t {
	case "Extension", "RingGroup", "Queue", "IVR", "VoiceMail":
		return t
	case "":
		return ""
	}
	return "none"
}
