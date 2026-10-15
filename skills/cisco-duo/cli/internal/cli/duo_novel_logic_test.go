// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
	"cisco-duo-pp-cli/internal/store"
)

func i64(v int64) *int64   { return &v }
func str(v string) *string { return &v }

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func TestDuoUnwrap(t *testing.T) {
	resp, meta, err := duoUnwrap(json.RawMessage(`{"stat":"OK","response":[{"a":1}],"metadata":{"next_offset":100}}`))
	if err != nil || string(resp) != `[{"a":1}]` || !strings.Contains(string(meta), "100") {
		t.Fatalf("ok envelope: %s %s %v", resp, meta, err)
	}
	if _, _, err := duoUnwrap(json.RawMessage(`{"stat":"FAIL","code":40002,"message":"Invalid request parameters","message_detail":"username"}`)); err == nil || !strings.Contains(err.Error(), "40002") || !strings.Contains(err.Error(), "username") {
		t.Fatalf("fail envelope should error with code and detail, got %v", err)
	}
	raw := json.RawMessage(`[1,2]`)
	if got, _, _ := duoUnwrap(raw); string(got) != "[1,2]" {
		t.Fatalf("bare body should pass through: %s", got)
	}
}

func TestEffectiveStatusGroupOverride(t *testing.T) {
	u := duoUser{Status: "active", Groups: []duoGroup{{GroupID: "DG1", Name: "Contractors", Status: "Disabled"}}}
	if st, _, g := effectiveStatus(u); st != "disabled" || g == nil || g.GroupID != "DG1" {
		t.Fatalf("disabled group must override: %s %+v", st, g)
	}
	u = duoUser{Status: "locked out", Groups: []duoGroup{{GroupID: "DG1", Name: "Off", Status: "Disabled"}}}
	if st, _, g := effectiveStatus(u); st != "locked out" || g == nil {
		t.Fatalf("locked out takes precedence but the disabled group is still reported: %s %+v", st, g)
	}
	if s := planFix(duoUser{UserID: "DU1", Status: "locked out", Groups: u.Groups}, fixOptions{}); len(s) < 2 || s[1].Step != "group-override" {
		t.Fatalf("a disabled group must block even when the user is locked out: %+v", s)
	}
	u = duoUser{Status: "locked out", Groups: []duoGroup{{Name: "VIP", Status: "Bypass"}}}
	if st, basis, _ := effectiveStatus(u); st != "locked out" || basis != "user status" {
		t.Fatalf("lockout wins over bypass group: %s %s", st, basis)
	}
	u = duoUser{Status: "active", Groups: []duoGroup{{Name: "VIP", Status: "Bypass"}}}
	if st, _, g := effectiveStatus(u); st != "bypass" || g == nil {
		t.Fatalf("bypass group applies to active user: %s", st)
	}
	if st, _, g := effectiveStatus(duoUser{Status: "active"}); st != "active" || g != nil {
		t.Fatalf("plain active: %s", st)
	}
}

func lockedUser(reason string) duoUser {
	return duoUser{UserID: "DU3RP9I2WOC59VZX672N", Username: "jdoe", Email: "jdoe@example.com", Status: "locked out",
		LockoutReason: str(reason), IsEnrolled: true,
		Phones: []duoPhone{{PhoneID: "DPFZRS9FB0D46QFTM899", Number: "15555550100", Platform: "Apple iOS", Type: "Mobile", Activated: true, Capabilities: []string{"push", "sms"}}}}
}

func TestBuildTriageNextActions(t *testing.T) {
	v := buildTriage(lockedUser("Failed Attempts"), nil, nil, nil, "DA9VZOC5X63I2W72NRP9", now)
	if len(v.NextActions) == 0 || !strings.Contains(v.NextActions[0].Command, "lockout fix DU3RP9I2WOC59VZX672N --account-id DA9VZOC5X63I2W72NRP9 --apply") {
		t.Fatalf("first action should unlock: %+v", v.NextActions)
	}
	v = buildTriage(lockedUser("Admin disabled"), nil, nil, nil, "", now)
	if !strings.Contains(v.NextActions[0].Command, "--force") {
		t.Fatalf("trust monitor hold needs --force: %+v", v.NextActions[0])
	}
	healthy := lockedUser("")
	healthy.Status, healthy.LockoutReason = "active", nil
	v = buildTriage(healthy, nil, nil, nil, "", now)
	if len(v.NextActions) != 0 || !strings.Contains(v.Findings[0], "No lockout detected") {
		t.Fatalf("healthy user should have no actions: %+v %v", v.NextActions, v.Findings)
	}
	codes := []duoBypassCode{{BypassCodeID: "old", Expiration: i64(now.Add(-time.Hour).Unix())}, {BypassCodeID: "live"}}
	v = buildTriage(healthy, codes, nil, nil, "", now)
	if len(v.BypassCodes) != 1 || v.BypassCodes[0].BypassCodeID != "live" {
		t.Fatalf("expired codes must be dropped: %+v", v.BypassCodes)
	}
	zero := 0
	sms := []triageFailure{{Reason: "no_response", Factor: "sms_passcode", Count: 3}}
	v = buildTriage(lockedUser("Failed Attempts"), nil, sms, &zero, "Acme Corp", now)
	found := false
	for _, a := range v.NextActions {
		found = found || strings.Contains(a.Command, "set-telephony-credits --account-id 'Acme Corp' --credits")
	}
	if !found {
		t.Fatalf("subaccount with zero credits and SMS failures should suggest moving credits: %+v", v.NextActions)
	}
	v = buildTriage(lockedUser("Failed Attempts"), nil, sms, &zero, "", now)
	for _, a := range v.NextActions {
		if strings.Contains(a.Command, "<") || strings.Contains(a.Command, "set-telephony-credits") {
			t.Fatalf("parent account must not get a subaccount-only or placeholder command: %+v", a)
		}
	}
}

func TestPlanFixGuards(t *testing.T) {
	soleOwner := map[string][]duoPhoneUser{"DPFZRS9FB0D46QFTM899": {{UserID: "DU3RP9I2WOC59VZX672N"}}}
	steps := planFix(lockedUser("Failed Attempts"), fixOptions{bypass: true, reactivate: true, validSecs: 3600, owners: soleOwner})
	if steps[0].Step != "unlock" || steps[0].Status != "planned" || steps[1].Step != "bypass-code" || steps[2].Status != "planned" {
		t.Fatalf("expected unlock+bypass+reactivate planned: %+v", steps)
	}
	if !strings.HasSuffix(steps[2].Path, "/send_sms_activation") {
		t.Fatalf("reactivate should text by default: %s", steps[2].Path)
	}
	if s := planFix(lockedUser("Admin disabled"), fixOptions{}); s[0].Status != "blocked" {
		t.Fatalf("Admin disabled must block without --force: %+v", s[0])
	}
	groupHeld := lockedUser("")
	groupHeld.Status = "active"
	groupHeld.Groups = []duoGroup{{GroupID: "DG1", Name: "Terminated", Status: "Disabled"}}
	noPhone := lockedUser("Admin API disabled")
	noPhone.Phones = nil
	notActivated := lockedUser("Admin disabled")
	notActivated.Phones[0].Activated = false
	disabledNotActivated := lockedUser("")
	disabledNotActivated.Status = "disabled"
	disabledNotActivated.Phones[0].Activated = false
	quarantined := lockedUser("Admin API disabled")
	quarantined.Phones[0].Quarantined = true
	notEnrolledGroup := lockedUser("Not enrolled")
	notEnrolledGroup.IsEnrolled = false
	notEnrolledGroup.Groups = []duoGroup{{GroupID: "DG1", Name: "Terminated", Status: "Disabled"}}
	trashed := lockedUser("")
	trashed.Status = "pending deletion"
	trashed.Phones[0].Activated = false
	held := []duoUser{lockedUser("Admin disabled"), lockedUser("Admin API disabled"), func() duoUser { d := lockedUser(""); d.Status = "disabled"; return d }(), groupHeld, noPhone, notActivated, disabledNotActivated, quarantined, notEnrolledGroup, trashed}
	for _, u := range held {
		for _, st := range planFix(u, fixOptions{bypass: true, reactivate: true, validSecs: 3600}) {
			if st.Status != "blocked" && st.Step != "unlock" {
				t.Fatalf("a held user (%s %s) must get no bypass code or activation without --force: %+v", u.Status, u.lockoutReason(), st)
			}
		}
		for _, a := range buildTriage(u, nil, nil, nil, "", now).NextActions {
			if strings.Contains(a.Command, "--bypass") || strings.Contains(a.Command, "--reactivate-phone") || strings.Contains(a.Command, "phone-swap") ||
				strings.Contains(a.Command, "set-quarantine-status") || strings.Contains(a.Command, "users enroll") || strings.Contains(a.Command, "groups update") {
				t.Fatalf("triage must not suggest a bypass code, activation or phone for a held user (%s %s): %+v", u.Status, u.lockoutReason(), a)
			}
		}
		sw := planSwap(u, swapOptions{number: "+15559998888", platform: "Generic Smartphone"}, nil, nil)
		if sw[0].Step != "hold" || sw[0].Status != "blocked" {
			t.Fatalf("phone-swap must refuse a held user (%s %s) without --force: %+v", u.Status, u.lockoutReason(), sw)
		}
		if sw = planSwap(u, swapOptions{number: "+15559998888", platform: "Generic Smartphone", force: true}, nil, nil); sw[0].Step == "hold" {
			t.Fatalf("phone-swap --force should lift the hold: %+v", sw)
		}
	}
	quarActive := lockedUser("")
	quarActive.Status = "active"
	quarActive.Phones[0].Quarantined = true
	for _, a := range buildTriage(quarActive, nil, nil, nil, "", now).NextActions {
		if strings.Contains(a.Command, "--reactivate-phone") {
			t.Fatalf("a quarantined phone cannot be reactivated, so triage must not suggest it: %+v", a)
		}
	}
	bypassGroup := lockedUser("")
	bypassGroup.Status = "active"
	bypassGroup.Groups = []duoGroup{{GroupID: "DG2", Name: "Execs", Status: "Bypass"}}
	for _, a := range buildTriage(bypassGroup, nil, nil, nil, "", now).NextActions {
		if strings.Contains(a.Command, "--bypass") {
			t.Fatalf("a user who already skips MFA must not be offered a bypass code: %+v", a)
		}
	}
	tokenUser := lockedUser("")
	tokenUser.Status, tokenUser.Phones = "active", nil
	if v := buildTriage(tokenUser, nil, nil, nil, "", now); len(v.NextActions) != 0 || !strings.Contains(v.Findings[0], "factor other than a phone") {
		t.Fatalf("an enrolled user with no phone (token or WebAuthn) is healthy: %+v %v", v.NextActions, v.Findings)
	}
	if s := planFix(lockedUser("Admin API disabled"), fixOptions{}); s[0].Status != "blocked" {
		t.Fatalf("an Admin API lock must block the unlock without --force: %+v", s[0])
	}
	if s := planFix(lockedUser("Admin disabled"), fixOptions{force: true, bypass: true, validSecs: 3600}); s[0].Status != "planned" || s[1].Status != "planned" {
		t.Fatalf("--force should allow: %+v", s[0])
	}
	active := lockedUser("")
	active.Status = "active"
	active.Groups = []duoGroup{{GroupID: "DG1", Name: "Off", Status: "Disabled"}}
	s := planFix(active, fixOptions{})
	if s[0].Status != "skipped" || len(s) < 2 || s[1].Step != "group-override" || s[1].Status != "blocked" {
		t.Fatalf("group override should be reported blocked: %+v", s)
	}
	landline := lockedUser("Failed Attempts")
	landline.Phones[0].Type, landline.Phones[0].Platform = "Landline", "Unknown"
	if s := planFix(landline, fixOptions{reactivate: true}); s[len(s)-1].Status != "blocked" {
		t.Fatalf("landline cannot be activated: %+v", s)
	}
}

func TestPlanSwap(t *testing.T) {
	u := lockedUser("")
	u.Phones = append(u.Phones, duoPhone{PhoneID: "DPOLD2", Number: "15555550199"})
	opt := swapOptions{number: "+15551234567", removeOld: true, deleteOld: true, platform: "Generic Smartphone"}
	owners := map[string][]duoPhoneUser{"DPFZRS9FB0D46QFTM899": {{UserID: u.UserID}}, "DPOLD2": {{UserID: u.UserID}, {UserID: "DUOTHER"}}}
	steps := planSwap(u, opt, nil, owners)
	order := make([]string, 0)
	got := map[string]string{}
	for _, s := range steps {
		order = append(order, s.Step)
		got[s.Step+":"+s.phoneID] = s.Status
	}
	if strings.Join(order[:3], ",") != "create-phone,associate,activate" {
		t.Fatalf("activation must precede retirement: %v", order)
	}
	if got["disassociate-old:DPFZRS9FB0D46QFTM899"] != "deferred" || got["delete-old:DPFZRS9FB0D46QFTM899"] != "deferred" {
		t.Fatalf("retirement must wait for activation: %+v", steps)
	}
	// Second run: the new phone is on the user and activated.
	u2 := u
	u2.Phones = append(append([]duoPhone{}, u.Phones...), duoPhone{PhoneID: "DPNEW", Number: "15551234567", Type: "Mobile", Platform: "Apple iOS", Activated: true})
	steps = planSwap(u2, opt, nil, owners)
	got = map[string]string{}
	for _, s := range steps {
		got[s.Step+":"+s.phoneID] = s.Status
	}
	if got["activate:DPNEW"] != "skipped" || got["delete-old:DPFZRS9FB0D46QFTM899"] != "planned" || got["delete-old:DPOLD2"] != "skipped" {
		t.Fatalf("after activation, sole-owner phone deleted and shared kept: %+v", steps)
	}
	steps = planSwap(u, swapOptions{number: "+1 (555) 555-0100", platform: "Generic Smartphone"}, nil, nil)
	if steps[0].Status != "skipped" || steps[0].phoneID != "DPFZRS9FB0D46QFTM899" || steps[len(steps)-1].Step != "activate" {
		t.Fatalf("existing number on user should be reused and activated: %+v", steps)
	}
	unknown := &duoPhone{PhoneID: "DPUNK", Number: "15551234567", Type: "Unknown", Platform: "Unknown"}
	owners["DPUNK"] = nil // registered to the number, attached to no user yet
	steps = planSwap(u, opt, unknown, owners)
	if steps[1].Step != "update-phone" || steps[1].Status != "planned" {
		t.Fatalf("unknown reused phone must be updated before activation: %+v", steps)
	}
	landline := &duoPhone{PhoneID: "DPLAND", Number: "15551234567", Type: "Landline", Platform: "Unknown"}
	steps = planSwap(u, opt, landline, owners)
	if steps[len(steps)-1].Status != "blocked" || len(steps) != 2 {
		t.Fatalf("landline must block before any retirement step: %+v", steps)
	}
	// A phone shared with another user (or whose users are unknown) is never
	// attached or activated without --force: activation binds Duo Mobile for
	// every user on it to whoever completes it.
	shared := &duoPhone{PhoneID: "DPSHARED", Number: "15551234567", Type: "Mobile", Platform: "Apple iOS"}
	owners["DPSHARED"] = []duoPhoneUser{{UserID: "DUVICTIM"}}
	for _, o := range []map[string][]duoPhoneUser{owners, {}} {
		steps = planSwap(u, swapOptions{number: "+15551234567", url: true, platform: "Generic Smartphone"}, shared, o)
		if last := steps[len(steps)-1]; last.Step != "shared-phone" || last.Status != "blocked" || firstBlocked(steps) == nil {
			t.Fatalf("a shared or unconfirmed phone must block phone-swap without --force: %+v", steps)
		}
	}
	if steps = planSwap(u, swapOptions{number: "+15551234567", platform: "Generic Smartphone", force: true}, shared, owners); firstBlocked(steps) != nil || !strings.Contains(steps[len(steps)-1].Detail, "--force overrides") {
		t.Fatalf("--force lifts the shared-phone block but the plan still names it: %+v", steps)
	}
	inactive := lockedUser("Failed Attempts")
	for _, o := range []map[string][]duoPhoneUser{{"DPFZRS9FB0D46QFTM899": {{UserID: inactive.UserID}, {UserID: "DUVICTIM"}}}, nil} {
		st := planFix(inactive, fixOptions{reactivate: true, url: true, owners: o})
		if st[len(st)-1].Step != "reactivate-phone" || st[len(st)-1].Status != "blocked" {
			t.Fatalf("lockout fix must not reactivate a shared or unconfirmed phone without --force: %+v", st)
		}
	}
	// A held user on a shared phone: the hold preview names the sharing too,
	// and the --force plan repeats it, so approving --force is never blind.
	heldShared := lockedUser("Admin disabled")
	heldShared.Phones[0].Activated = false
	sharedOwners := map[string][]duoPhoneUser{"DPFZRS9FB0D46QFTM899": {{UserID: heldShared.UserID}, {UserID: "DUVICTIM"}}}
	if st := planFix(heldShared, fixOptions{reactivate: true, owners: sharedOwners}); !strings.Contains(st[len(st)-1].Detail, "shared with 1 other user") {
		t.Fatalf("the hold preview must also show the sharing: %+v", st[len(st)-1])
	}
	if st := planFix(heldShared, fixOptions{reactivate: true, force: true, owners: sharedOwners}); st[len(st)-1].Status != "planned" || !strings.Contains(st[len(st)-1].Detail, "--force overrides") {
		t.Fatalf("the --force plan must name the sharing it overrides: %+v", st[len(st)-1])
	}
	if !samePhoneNumber("15555550100", "+1 (555) 555-0100") || samePhoneNumber("15555550100", "+15555550101") {
		t.Fatal("samePhoneNumber")
	}
	if samePhoneNumber("14792234567", "+4792234567") {
		t.Fatal("an international number must never match a different US number")
	}
	if !e164.MatchString("+4792234567") || e164.MatchString("5551234567") || e164.MatchString("+0123456789") {
		t.Fatal("e164 validation")
	}
}

func TestCountRisk(t *testing.T) {
	users := []duoUser{
		{Status: "bypass"}, {Status: "locked out"}, {Status: "active", IsEnrolled: false},
		{Status: "active", IsEnrolled: true, Groups: []duoGroup{{Status: "Bypass"}}},
		{Status: "pending deletion"}, {Status: "disabled", Groups: []duoGroup{{Status: "Disabled"}}},
	}
	codes := []duoBypassCode{{}, {Expiration: i64(now.Add(time.Hour).Unix()), ReuseCount: i64(1)},
		{Expiration: i64(now.Add(-time.Hour).Unix())}, {Expiration: i64(now.Add(time.Hour).Unix())}}
	r := countRisk(users, codes, now)
	if r.Bypass != 1 || r.LockedOut != 1 || r.Unenrolled != 1 || r.InBypassGroup != 1 || r.PendingDeletion != 1 || r.Disabled != 1 || r.InDisabledGroup != 1 {
		t.Fatalf("user counts wrong: %+v", r)
	}
	if r.StandingCodes != 2 {
		t.Fatalf("standing codes = no expiry or unlimited reuse, expired ignored: %d", r.StandingCodes)
	}
	if riskScore(r) != 3*(1+1+2)+2*1+1 {
		t.Fatalf("risk score %d", riskScore(r))
	}
	empty := countRisk(nil, nil, now)
	if empty.Users != 0 || riskScore(empty) != 0 {
		t.Fatal("empty tenant must score 0")
	}
}

func TestClassifyBypass(t *testing.T) {
	users := []duoUser{{UserID: "DU1", Username: "a", Status: "bypass"}, {UserID: "DU2", Username: "b", Status: "active", Groups: []duoGroup{{GroupID: "DG", Name: "VIP", Status: "Bypass"}}}, {UserID: "DU3", Username: "c", Status: "active"}}
	codes := []duoBypassCode{
		{BypassCodeID: "c1", AdminEmail: "x@example.com", User: &duoPhoneUser{UserID: "DU3", Username: "c"}},
		{BypassCodeID: "c2", Expiration: i64(now.Add(time.Hour).Unix()), User: &duoPhoneUser{Username: "c"}},
		{BypassCodeID: "c3", Expiration: i64(now.Add(time.Hour).Unix()), ReuseCount: i64(1), Created: now.Add(-40 * 24 * time.Hour).Unix(), User: &duoPhoneUser{Username: "c"}},
		{BypassCodeID: "c4", Expiration: i64(now.Add(time.Hour).Unix()), ReuseCount: i64(1), Created: now.Add(-time.Hour).Unix()},
		{BypassCodeID: "c5", Expiration: i64(now.Add(-time.Hour).Unix())},
	}
	rows := classifyBypass(users, codes, 30*24*time.Hour, now)
	paths := map[string]bool{}
	for _, r := range rows {
		paths[r.Path+":"+r.Username] = true
	}
	for _, want := range []string{"user-status:a", "group:b", "code-no-expiry:c", "code-unlimited-reuse:c", "code-old:c"} {
		if !paths[want] {
			t.Fatalf("missing %s in %+v", want, rows)
		}
	}
	if len(rows) != 5 {
		t.Fatalf("fresh limited code and expired code must not appear: %+v", rows)
	}
	if got := classifyBypass([]duoUser{{Username: "z", Status: "active"}}, nil, 0, now); len(got) != 0 {
		t.Fatalf("clean tenant must be empty: %+v", got)
	}
}

func TestStaleFindings(t *testing.T) {
	old := now.Add(-200 * 24 * time.Hour).Unix()
	recent := now.Add(-time.Hour).Unix()
	users := []duoUser{
		{UserID: "DU1", Username: "new", Status: "active", Created: recent},
		{UserID: "DU2", Username: "ghost", Status: "active", Created: old},
		{UserID: "DU3", Username: "busy", Status: "active", IsEnrolled: true, Created: old, LastLogin: i64(recent)},
		{UserID: "DU4", Username: "gone", Status: "pending deletion", Created: old},
	}
	admins := []duoAdmin{{AdminID: "DE1", Email: "a@example.com", Status: "Expired"}, {AdminID: "DE2", Email: "b@example.com", Status: "Active", LastLogin: i64(old)},
		{AdminID: "DE3", Email: "c@example.com", Status: "Active", Created: i64(old)}, {AdminID: "DE4", Email: "d@example.com", Status: "Active", Created: i64(recent)}}
	phones := []duoPhone{{PhoneID: "DP1", Number: "1", Activated: true, LastSeen: flexTime(now.Add(-100 * 24 * time.Hour).Format("2006-01-02T15:04:05"))}, {PhoneID: "DP2", Activated: false, LastSeen: flexTime("2001-01-01T00:00:00")}}
	enabled, _ := parseChecks("")
	rows := staleFindingsFor(users, admins, phones, 90, enabled, now)
	got := map[string]bool{}
	for _, r := range rows {
		got[r.Check+":"+r.ID] = true
	}
	for _, want := range []string{"unenrolled:DU2", "inactive:DU2", "pending-deletion:DU4", "admin-expired:DE1", "admin-inactive:DE2", "admin-inactive:DE3", "phone-unseen:DP1"} {
		if !got[want] {
			t.Fatalf("missing %s in %+v", want, rows)
		}
	}
	if got["unenrolled:DU1"] || got["inactive:DU3"] || got["phone-unseen:DP2"] || got["admin-inactive:DE4"] {
		t.Fatalf("false positives: %+v", rows)
	}
	only, err := parseChecks("locked")
	if err != nil || len(staleFindingsFor(users, admins, phones, 90, only, now)) != 0 {
		t.Fatal("check filter should limit findings")
	}
	if _, err := parseChecks("bogus"); err == nil {
		t.Fatal("unknown check must error")
	}
}

func TestRollupFailures(t *testing.T) {
	mk := func(user, reason, result, ts, ip string) authLogEvent {
		e := authLogEvent{Result: result, Reason: reason, Factor: "duo_push", ISOTimestamp: ts}
		e.User.Name, e.Application.Name, e.AccessDevice.IP = user, "VPN", ip
		return e
	}
	events := []authLogEvent{
		mk("jdoe", "locked_out", "denied", "2026-10-13T10:00:00", "1.1.1.1"),
		mk("jdoe", "locked_out", "denied", "2026-10-13T09:00:00", "2.2.2.2"),
		mk("asmith", "user_marked_fraud", "fraud", "2026-10-13T08:00:00", "3.3.3.3"),
		mk("jdoe", "user_approved", "success", "2026-10-13T11:00:00", "1.1.1.1"),
	}
	rows, failures := rollupFailures(events, 0)
	if failures != 3 || len(rows) != 2 || rows[0].User != "jdoe" || rows[0].Count != 2 || len(rows[0].IPs) != 2 {
		t.Fatalf("rollup wrong: %d %+v", failures, rows)
	}
	if rows[0].FirstSeen != "2026-10-13T09:00:00" || rows[0].LastSeen != "2026-10-13T10:00:00" || rows[0].Flag == "" {
		t.Fatalf("first/last/flag wrong: %+v", rows[0])
	}
	if top, _ := rollupFailures(events, 1); len(top) != 1 {
		t.Fatal("--top not applied")
	}
	if none, n := rollupFailures(events[3:], 0); len(none) != 0 || n != 0 {
		t.Fatal("success-only window must be empty")
	}
}

func TestSelectChaseTargets(t *testing.T) {
	old := now.Add(-10 * 24 * time.Hour).Unix()
	users := []duoUser{
		{UserID: "DU1", Username: "a", Email: "a@example.com", Status: "active", Created: old},
		{UserID: "DU2", Username: "b", Email: "b@example.com", Status: "active", Created: old, IsEnrolled: true},
		{UserID: "DU3", Username: "c", Email: "", Status: "active", Created: old},
		{UserID: "DU4", Username: "d", Email: "d@example.com", Status: "active", Created: now.Unix()},
		{UserID: "DU5", Username: "e", Email: "e@example.com", Status: "disabled", Created: old},
		{UserID: "DU6", Username: "f", Email: "f@example.com", Status: "active", Created: old, HasPassword: true},
		{UserID: "DU7", Username: "g", Email: "g@example.com", Status: "active", Created: old - 100},
		{UserID: "DU8", Username: "h", Email: "h@example.com", Status: "active", Created: old - 200,
			Groups: []duoGroup{{GroupID: "DG1", Name: "Terminated", Status: "Disabled"}}},
	}
	got := selectChaseTargets(users, 72*time.Hour, 0, now)
	if len(got) != 2 || got[0].Username != "g" || got[1].Username != "a" {
		t.Fatalf("expected oldest-first a,g only: %+v", got)
	}
	if len(selectChaseTargets(users, 72*time.Hour, 1, now)) != 1 {
		t.Fatal("limit not applied")
	}
}

func TestParseUserLimit(t *testing.T) {
	c, l := parseUserLimit(json.RawMessage(`{"current_users_count":42,"user_limit":"Not set"}`))
	if c == nil || *c != 42 || l != nil {
		t.Fatalf("Not set should be nil limit: %v %v", c, l)
	}
	c, l = parseUserLimit(json.RawMessage(`{"current_users_count":5,"user_limit":10}`))
	if l == nil || *l != 10 || *c != 5 {
		t.Fatal("numeric limit")
	}
}

func TestFlexTimeAcceptsNumberOrString(t *testing.T) {
	var p duoPhone
	if err := json.Unmarshal([]byte(`{"phone_id":"DP1","last_seen":1728064132}`), &p); err != nil || string(p.LastSeen) != "1728064132" {
		t.Fatalf("numeric last_seen: %v %q", err, p.LastSeen)
	}
	if ts, ok := duoParseTime(string(p.LastSeen)); !ok || ts.Year() != 2024 {
		t.Fatalf("numeric parse: %v", ts)
	}
	if err := json.Unmarshal([]byte(`{"last_seen":"2025-04-16T21:42:03"}`), &p); err != nil || string(p.LastSeen) != "2025-04-16T21:42:03" {
		t.Fatalf("string last_seen: %v %q", err, p.LastSeen)
	}
	if err := json.Unmarshal([]byte(`{"last_seen":null}`), &p); err != nil || p.LastSeen != "" {
		t.Fatalf("null last_seen: %v", err)
	}
}

func TestSubaccountDataDirIsolation(t *testing.T) {
	base := t.TempDir()
	t.Setenv(duoDataDirEnv, "")
	t.Setenv(duoParentDataDirEnv, "")
	userDataDir, inheritedBase = base, ""
	defer func() { userDataDir, inheritedBase = "", "" }()
	if err := pinSubaccountDataDir("da9vzoc5x63i2w72nrp9"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(duoDataDirEnv); got != filepath.Join(base, "subaccounts", "DA9VZOC5X63I2W72NRP9") {
		t.Fatalf("ID pin dir: %s", got)
	}
	if os.Getenv(duoParentDataDirEnv) != base {
		t.Fatalf("parent base must be exported for subprocesses: %s", os.Getenv(duoParentDataDirEnv))
	}
	a, b := subaccountDirName("Acme / East"), subaccountDirName("Acme East")
	if a == b || !strings.HasPrefix(a, "name-acme-east-") || subaccountDirName("ACME / east") != a {
		t.Fatalf("names must not collide but must case-fold: %s %s", a, b)
	}
	if subaccountDirName("Café") == subaccountDirName("日本") {
		t.Fatal("non-ASCII names must not collide")
	}
	// A subprocess inherits the parent base and must not nest twice.
	userDataDir, inheritedBase = filepath.Join(base, "subaccounts", "DA9VZOC5X63I2W72NRP9"), base
	if err := pinSubaccountDataDir("DA9VZOC5X63I2W72NRP9"); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(duoDataDirEnv); got != filepath.Join(base, "subaccounts", "DA9VZOC5X63I2W72NRP9") {
		t.Fatalf("child must reuse the parent base, got %s", got)
	}
	if err := pinSubaccountDataDir(""); err != nil || os.Getenv(duoDataDirEnv) != base {
		t.Fatalf("clearing the pin must restore the parent dir: %s", os.Getenv(duoDataDirEnv))
	}
}

func TestShellQuoteAndTriageCommands(t *testing.T) {
	for _, v := range []string{"=ls", "a=b", "^x", "~x", "a\x00b", "é"} {
		if shellQuote(v) != "'"+v+"'" {
			t.Fatalf("shellQuote(%q) = %s, want single-quoted", v, shellQuote(v))
		}
	}
	if shellQuote("DU123") != "DU123" || shellQuote("Acme Corp") != "'Acme Corp'" || shellQuote("it's") != `'it'\''s'` {
		t.Fatalf("shellQuote: %s %s %s", shellQuote("DU123"), shellQuote("Acme Corp"), shellQuote("it's"))
	}
	v := buildTriage(lockedUser("Failed Attempts"), nil, nil, nil, "Acme Corp", now)
	if !strings.Contains(v.NextActions[0].Command, "lockout fix DU3RP9I2WOC59VZX672N --account-id 'Acme Corp' --apply") {
		t.Fatalf("suggested command must use user_id and a quoted account: %s", v.NextActions[0].Command)
	}
}

func TestQuarantineGuards(t *testing.T) {
	u := lockedUser("Failed Attempts")
	u.Phones[0].Quarantined = true
	if s := planFix(u, fixOptions{reactivate: true}); s[len(s)-1].Status != "blocked" {
		t.Fatalf("quarantined phone must not be reactivated: %+v", s)
	}
	v := buildTriage(u, nil, nil, nil, "", now)
	found := false
	for _, a := range v.NextActions {
		found = found || strings.Contains(a.Command, "set-quarantine-status DPFZRS9FB0D46QFTM899 --quarantine-status false")
	}
	if !found {
		t.Fatalf("triage must suggest releasing quarantine: %+v", v.NextActions)
	}
	u2 := lockedUser("")
	u2.Phones = append(u2.Phones, duoPhone{PhoneID: "DPNEW", Number: "15551234567", Type: "Mobile", Platform: "Apple iOS", Activated: true, Quarantined: true})
	steps := planSwap(u2, swapOptions{number: "+15551234567", removeOld: true, platform: "Generic Smartphone"}, nil, nil)
	if steps[len(steps)-1].Status != "blocked" {
		t.Fatalf("a quarantined replacement must block before retiring old phones: %+v", steps)
	}
}

func TestDecodeAllCount(t *testing.T) {
	got, bad := decodeAllCount[duoUser]([]json.RawMessage{json.RawMessage(`{"user_id":"DU1"}`), json.RawMessage(`{"user_id":`), json.RawMessage(`{"created":"not-a-number"}`)})
	if len(got) != 1 || bad != 2 {
		t.Fatalf("decodeAllCount: %d ok %d bad", len(got), bad)
	}
}

// TestSyncHintCarriesPinnedAccount guards the audit 'hint:' lines: with a
// subaccount pinned, the suggested sync must carry --account-id or following
// it fills the parent's store and the subaccount audit stays unsynced.
func TestSyncHintCarriesPinnedAccount(t *testing.T) {
	t.Setenv(client.DuoAccountEnv, "")
	if got, want := syncHint([]string{"users", "bypass_codes"}), "cisco-duo-cli sync --resources users,bypass_codes"; got != want {
		t.Fatalf("unpinned syncHint = %q, want %q", got, want)
	}
	t.Setenv(client.DuoAccountEnv, "DA9VZOC5X63I2W72NRP9")
	if got, want := syncHint([]string{"users"}), "cisco-duo-cli sync --resources users --account-id DA9VZOC5X63I2W72NRP9"; got != want {
		t.Fatalf("pinned syncHint = %q, want %q", got, want)
	}
	t.Setenv(client.DuoAccountEnv, "Acme Corp")
	if got := syncHint([]string{"users"}); !strings.HasSuffix(got, " --account-id 'Acme Corp'") {
		t.Fatalf("pinned-by-name syncHint = %q, want a shell-quoted name", got)
	}
}

func TestNameStoreBoundToResolvedAccount(t *testing.T) {
	base := t.TempDir()
	t.Setenv(duoDataDirEnv, "")
	t.Setenv(duoParentDataDirEnv, "")
	t.Setenv(client.DuoAccountEnv, "Acme Corp")
	userDataDir, inheritedBase = base, ""
	defer func() { userDataDir, inheritedBase = "", "" }()
	if err := pinSubaccountDataDir("Acme Corp"); err != nil {
		t.Fatal(err)
	}
	if err := bindNameStore("Acme Corp", "DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("first resolution binds the store: %v", err)
	}
	if err := bindNameStore("acme corp", "DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("same account again is fine: %v", err)
	}
	if err := bindNameStore("Acme Corp", "DBBBBBBBBBBBBBBBBBBB"); err == nil || !strings.Contains(err.Error(), "DBBBBBBBBBBBBBBBBBBB") {
		t.Fatalf("a name that moved to another subaccount must refuse the old store: %v", err)
	}
	if err := bindNameStore("Other Co", "DBBBBBBBBBBBBBBBBBBB"); err != nil {
		t.Fatalf("a name that is not the pin is not this store's business: %v", err)
	}
	// An ID pin binds too: if that account is deleted and another account is
	// later NAMED with the old ID string, the old mirror is refused.
	t.Setenv(client.DuoAccountEnv, "DAAAAAAAAAAAAAAAAAAA")
	if err := pinSubaccountDataDir("DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	if err := bindNameStore("DAAAAAAAAAAAAAAAAAAA", "DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("an ID pin binds its store: %v", err)
	}
	if err := bindNameStore("DAAAAAAAAAAAAAAAAAAA", "DBBBBBBBBBBBBBBBBBBB"); err == nil {
		t.Fatal("an ID string that now names another account must refuse the old store")
	}
	// A subaccount NAMED like an ID ("DCCCC...") that resolves to another
	// account is still a name: its store is bound and checked like any name.
	t.Setenv(client.DuoAccountEnv, "DCCCCCCCCCCCCCCCCCCC")
	if err := pinSubaccountDataDir("DCCCCCCCCCCCCCCCCCCC"); err != nil {
		t.Fatal(err)
	}
	if err := bindNameStore("DCCCCCCCCCCCCCCCCCCC", "DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("first resolution binds: %v", err)
	}
	if err := bindNameStore("DCCCCCCCCCCCCCCCCCCC", "DBBBBBBBBBBBBBBBBBBB"); err == nil {
		t.Fatal("an ID-shaped name that moved to another account must refuse its old store")
	}
}

func TestSyncGapsCountsAnInterruptedSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveSyncState("users", "", 10); err != nil { // a completed sync
		t.Fatal(err)
	}
	if err := db.SaveSyncStateAt("bypass_codes", "", 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("bypass_codes", "page-2", 1); err != nil { // a refresh that stopped part-way
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	var errOut strings.Builder
	cmd.SetErr(&errOut)
	gaps := syncGaps(cmd, db, 0, "users", "bypass_codes")
	if len(gaps) != 1 || gaps[0] != "bypass_codes" || !strings.Contains(errOut.String(), "stopped part-way") {
		t.Fatalf("an interrupted refresh must make the audit incomplete: gaps=%v hint=%q", gaps, errOut.String())
	}
}

func TestRefuseSyncParams(t *testing.T) {
	mk := func(flag, val string) *cobra.Command {
		root := &cobra.Command{Use: "cisco-duo-cli"}
		syncCmd := &cobra.Command{Use: "sync"}
		for _, f := range []string{"param", "global-param", "resource-param"} {
			syncCmd.Flags().StringArray(f, nil, "")
		}
		root.AddCommand(syncCmd)
		if err := syncCmd.Flags().Set(flag, val); err != nil {
			t.Fatal(err)
		}
		return syncCmd
	}
	for _, c := range [][2]string{{"param", "account_id=DB"}, {"global-param", "ACCOUNT_ID=DB"}, {"resource-param", "users:account_id=DB"}} {
		if refuseSyncParams(mk(c[0], c[1])) == nil {
			t.Fatalf("--%s %s must be refused", c[0], c[1])
		}
	}
	// A filter or a page size the paginator does not expect would leave the
	// mirror partial while sync marks it complete.
	for _, c := range [][2]string{{"resource-param", "users:username=jdoe"}, {"param", "status=active"}, {"param", "limit=1"}, {"global-param", "LIMIT=300"}} {
		if refuseSyncParams(mk(c[0], c[1])) == nil {
			t.Fatalf("--%s %s must be refused (partial mirror marked complete)", c[0], c[1])
		}
	}
}

func TestClaimStoreMarkerIsExclusive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subaccounts", "name-acme")
	marker := filepath.Join(dir, "account-id")
	var wg sync.WaitGroup
	seen := make([]string, 16) // what each claimant reads back right after claiming
	for i := 0; i < 16; i++ {
		id := "DAAAAAAAAAAAAAAAAAAA"
		if i%2 == 1 {
			id = "DBBBBBBBBBBBBBBBBBBB"
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			if err := claimStoreMarker(dir, marker, id); err != nil {
				t.Errorf("claim: %v", err)
			}
			b, _ := os.ReadFile(marker)
			seen[i] = strings.TrimSpace(string(b))
		}(i, id)
	}
	wg.Wait()
	for _, v := range seen {
		if v != seen[0] {
			t.Fatalf("racing claimants must all see one winner, saw %v", seen)
		}
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if v := strings.TrimSpace(string(got)); v != "DAAAAAAAAAAAAAAAAAAA" && v != "DBBBBBBBBBBBBBBBBBBB" {
		t.Fatalf("exactly one racing claim must win intact, got %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".account-id-*")); len(left) != 0 {
		t.Fatalf("temp files left behind: %v", left)
	}
}

// TestEmptyStoreMarker: on a filesystem without hard links the marker is
// created before it is written. A concurrent claimant that finishes within the
// wait is honored (not a false mismatch); a marker left empty is refused, never
// overwritten, because a delayed claimant may still write another account's ID.
func TestEmptyStoreMarker(t *testing.T) {
	base := t.TempDir()
	t.Setenv(duoDataDirEnv, "")
	t.Setenv(duoParentDataDirEnv, "")
	t.Setenv(client.DuoAccountEnv, "Acme Corp")
	userDataDir, inheritedBase = base, ""
	defer func() { userDataDir, inheritedBase = "", "" }()
	if err := pinSubaccountDataDir("Acme Corp"); err != nil {
		t.Fatal(err)
	}
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "account-id")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(marker, []byte("DAAAAAAAAAAAAAAAAAAA\n"), 0o600)
	}()
	if err := bindNameStore("Acme Corp", "DAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatalf("a claimant that finishes during the wait is not a mismatch: %v", err)
	}
	<-done
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bindNameStore("Acme Corp", "DBBBBBBBBBBBBBBBBBBB"); err == nil || !strings.Contains(err.Error(), "empty account-id marker") {
		t.Fatalf("an abandoned empty marker must be refused: %v", err)
	} else if !strings.Contains(err.Error(), "delete that directory") {
		t.Fatalf("the directory may hold another tenant's rows, so the advice must name it, not the marker: %v", err)
	}
	if got, _ := os.ReadFile(marker); len(got) != 0 {
		t.Fatalf("an empty marker must never be overwritten, got %q", got)
	}
}
