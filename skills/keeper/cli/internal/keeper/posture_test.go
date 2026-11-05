// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

import (
	"encoding/json"
	"testing"
)

func n(v int) json.Number { return json.Number(json.Number(itoa(v))) }

func itoa(v int) string { b, _ := json.Marshal(v); return string(b) }

func sampleInputs() Inputs {
	return Inputs{
		Company: Row{"company_id": n(4421), "plan": "business", "allocated": n(3), "active": n(4)},
		Users: []Row{
			{"email": "Admin@X.co", "name": "Ada", "status": "Active", "2fa_enabled": true, "roles": []any{"Admins"}},
			{"email": "bob@x.co", "status": "Active", "roles": []any{"Admins"}},
			{"email": "cy@x.co", "status": "Invited"},
			{"email": "di@x.co", "status": "Locked", "transfer_status": "Pending transfer"},
		},
		Roles: []Row{{"name": "Admins", "admin": true, "enforcements": map[string]any{}}},
		LastLogin: []Row{
			{"email": "admin@x.co", "last_login": "2026-10-01 10:00:00+00:00"},
			{"email": "bob@x.co", "last_login": "> 90 DAYS AGO"},
			{"email": "cy@x.co", "last_login": "N/A"},
		},
		Security: []Row{
			{"email": "admin@x.co", "weak": n(0), "reused": n(1), "securityScore": n(90)},
			{"email": "bob@x.co", "weak": n(4), "reused": n(2), "securityScore": n(40)},
		},
		BreachWatch:  []Row{{"email": "admin@x.co", "at_risk": n(0)}, {"email": "bob@x.co", "at_risk": n(2)}},
		InactiveDays: 90,
	}
}

func TestComputeFromVendorShapes(t *testing.T) {
	m, users := Compute(sampleInputs())
	if m.Users != 4 || m.Active != 2 || m.Invited != 1 || m.Locked != 1 {
		t.Fatalf("counts: %+v", m)
	}
	if m.TwoFAOn != 1 || m.TwoFAOff != 1 || m.TwoFACoverage == nil || *m.TwoFACoverage != 50 {
		t.Fatalf("2FA: %+v", m)
	}
	if m.Inactive != 1 || !users["bob@x.co"].Inactive || users["cy@x.co"].Inactive {
		t.Fatalf("inactive: %d %+v", m.Inactive, users)
	}
	if m.WeakTotal != 4 || m.UsersWithWeak != 1 || m.ReusedTotal != 3 || m.AvgSecurityScore == nil || *m.AvgSecurityScore != 65 {
		t.Fatalf("audit: %+v", m)
	}
	if m.BreachWatch != "active" || m.AtRiskTotal != 2 || m.UsersAtRisk != 1 {
		t.Fatalf("breachwatch: %+v", m)
	}
	if !m.SeatsOver {
		t.Fatal("4 active on 3 allocated is over")
	}
	if !users["bob@x.co"].Admin || users["bob@x.co"].TwoFAEnforced {
		t.Fatalf("role facts: %+v", users["bob@x.co"])
	}
	if users["di@x.co"].TransferStatus != "Pending transfer" {
		t.Fatal("transfer status lost")
	}
}

// --breachwatch is ignored when BreachWatch is off: rows carry no at_risk.
func TestBreachWatchNotActiveIsNotZeroRisk(t *testing.T) {
	in := sampleInputs()
	in.BreachWatch = []Row{{"email": "admin@x.co"}, {"email": "bob@x.co"}}
	m, _ := Compute(in)
	if m.BreachWatch != "not_active" {
		t.Fatalf("got %q", m.BreachWatch)
	}
}

func TestUnlimitedSeatsAreNeverOver(t *testing.T) {
	in := sampleInputs()
	in.Company["allocated"] = n(-1)
	if m, _ := Compute(in); m.SeatsOver {
		t.Fatal("unlimited (-1) counted as over-allocated")
	}
}

func TestDuplicateRoleNamesMergeConservatively(t *testing.T) {
	idx := roleIndex([]Row{
		{"name": "Ops", "admin": false, "enforcements": map[string]any{"require_two_factor": true}},
		{"name": "Ops", "admin": true, "enforcements": map[string]any{}},
	})
	if !idx.byName["Ops"].admin || idx.byName["Ops"].enforces {
		t.Fatalf("got %+v", idx.byName["Ops"])
	}
}

func TestDiffRaisesTheDocumentedEvents(t *testing.T) {
	prevIn := sampleInputs()
	prevM, prevU := Compute(prevIn)
	cur := sampleInputs()
	cur.Users[0] = Row{"email": "admin@x.co", "name": "Ada", "status": "Active", "roles": []any{"Admins"}} // 2FA off
	cur.Users = append(cur.Users, Row{"email": "new@x.co", "status": "Active"})
	cur.Users = cur.Users[:len(cur.Users)] // keep
	cur.Security[1] = Row{"email": "bob@x.co", "weak": n(6), "reused": n(2), "securityScore": n(30)}
	cur.BreachWatch[0] = Row{"email": "admin@x.co", "at_risk": n(1)}
	curM, curU := Compute(cur)
	evs := Diff("4421", prevM, curM, prevU, curU)
	got := map[string]bool{}
	for _, e := range evs {
		got[e.Type+" "+e.Resource] = true
	}
	for _, want := range []string{
		"posture.twofa_coverage_dropped 4421",
		"posture.weak_passwords_increased 4421",
		"posture.security_score_dropped 4421",
		"posture.breachwatch_at_risk_increased 4421",
		"user.twofa_disabled 4421/admin@x.co",
		"user.added 4421/new@x.co",
		"user.breachwatch_at_risk 4421/admin@x.co",
	} {
		if !got[want] {
			t.Errorf("missing %s (got %v)", want, got)
		}
	}
	if len(Diff("4421", prevM, prevM, prevU, prevU)) != 0 {
		t.Fatal("identical runs raised events")
	}
}

// A run with a different inactivity window never reports inactivity changes.
func TestDiffIgnoresInactivityAcrossDifferentWindows(t *testing.T) {
	a := sampleInputs()
	am, au := Compute(a)
	b := sampleInputs()
	b.InactiveDays = 30
	b.LastLogin[0] = Row{"email": "admin@x.co", "last_login": "> 30 DAYS AGO"}
	bm, bu := Compute(b)
	for _, e := range Diff("4421", am, bm, au, bu) {
		if e.Type == "posture.inactive_users_increased" || e.Type == "user.became_inactive" {
			t.Fatalf("raised %s across windows", e.Type)
		}
	}
}

// Codex r1 class duplicate-role-name-misattribution: Commander allows two
// roles with one name (commands/enterprise.py), so role membership is joined
// by role id (enterprise-info --users --verbose lists {role_id, role_name}).
func TestRoleFactsJoinByRoleID(t *testing.T) {
	jn := func(s string) json.Number { return json.Number(s) }
	in := Inputs{
		Users: []Row{
			{"user_id": jn("1"), "email": "ordinary@example.com", "status": "Active",
				"roles": []any{map[string]any{"role_id": "1", "role_name": "Ops"}}},
			{"user_id": jn("2"), "email": "admin@example.com", "status": "Active",
				"roles": []any{map[string]any{"role_id": "2", "role_name": "Ops"}}},
		},
		Roles: []Row{
			{"role_id": jn("1"), "name": "Ops", "admin": false, "enforcements": map[string]any{"require_two_factor": true}},
			{"role_id": jn("2"), "name": "Ops", "admin": true, "enforcements": map[string]any{}},
		},
	}
	_, users := Compute(in)
	if f := users["ordinary@example.com"]; f.Admin || !f.TwoFAEnforced {
		t.Errorf("ordinary user: admin %v enforced %v, want false true", f.Admin, f.TwoFAEnforced)
	}
	if f := users["admin@example.com"]; !f.Admin || f.TwoFAEnforced {
		t.Errorf("admin user: admin %v enforced %v, want true false", f.Admin, f.TwoFAEnforced)
	}
}
