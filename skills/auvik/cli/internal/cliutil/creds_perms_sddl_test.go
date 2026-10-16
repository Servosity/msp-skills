// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-wired (handfixes.json: windows-sddl-alias-sid-compare,
// windows-null-dacl-behind-flags): trustees in the token file's SDDL are
// compared by SID, so an alias that names the current user (the built-in
// Administrator renders as LA) is the owner, and an alias that names anyone
// else is not; and the DACL section is parsed exactly, so a NULL DACL is
// refused in the form Windows actually renders. Pure SDDL strings, so this
// runs on any OS.
package cliutil

import (
	"strings"
	"testing"
)

func TestEvalCredsSecurityResolvesSDDLAliases(t *testing.T) {
	const (
		builtinAdmin = "S-1-5-21-1-2-3-500"  // this machine's RID-500 account
		ordinaryUser = "S-1-5-21-1-2-3-1000" // a normal local account
		otherUser    = "S-1-5-21-1-2-3-2000"
	)
	// What ConvertStringSidToSid answers on a machine whose account domain is
	// S-1-5-21-1-2-3. DA (Domain Admins) is unresolvable off a domain.
	resolve := func(alias string) (string, bool) {
		switch alias {
		case "LA":
			return builtinAdmin, true
		case "LG":
			return "S-1-5-21-1-2-3-501", true
		}
		return "", false
	}
	cases := []struct {
		name    string
		me      string
		sddl    string
		wantErr string // "" = accepted; else a substring of the refusal
	}{
		// The built-in Administrator: the converter renders its own SID as LA.
		{"LA owner, only an LA ACE", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)", ""},
		{"LA owner, captured sandbox shape", builtinAdmin, "O:LAG:S-1-5-21-1-2-3-513D:PAI(A;OICI;FA;;;LA)", ""},
		{"elevated: BA owner, inherited SY/BA/LA ACEs", builtinAdmin, "O:BAG:S-1-5-21-1-2-3-513D:AI(A;ID;FA;;;SY)(A;ID;FA;;;BA)(A;ID;FA;;;LA)", ""},
		{"lowercase alias still resolves", builtinAdmin, "O:laD:(A;;FA;;;la)", ""},
		{"LA owner plus another user", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;" + otherUser + ")", "non-owner principal"},
		{"LA owner plus Everyone", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;WD)", "broad group"},
		{"LA owner plus BUILTIN Users", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;BU)", "broad group"},
		{"LA owner plus BUILTIN Users by SID", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;S-1-5-32-545)", "broad group"},
		{"LA owner plus local Guest", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;LG)", "non-owner principal"},
		{"LA owner plus unresolvable alias", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;DA)", "non-owner principal"},
		{"LA owner plus unknown alias", builtinAdmin, "O:LAD:PAI(A;;FA;;;LA)(A;;FR;;;ZZ)", "non-owner principal"},
		{"LA ACE beside an object-allow for another user", builtinAdmin, "O:LAD:(A;;FA;;;LA)(OA;;FR;;;" + otherUser + ")", "non-owner principal"},
		// Resolution is by SID, not a blanket LA allow: for anyone else, LA is
		// another principal.
		{"ordinary user, LA owner", ordinaryUser, "O:LAD:PAI(A;;FA;;;" + ordinaryUser + ")", "owner is not the current user"},
		{"ordinary user, LA ACE", ordinaryUser, "O:" + ordinaryUser + "D:PAI(A;;FA;;;" + ordinaryUser + ")(A;;FA;;;LA)", "non-owner principal"},
		// The ordinary-user case is unchanged.
		{"ordinary user, own SID", ordinaryUser, "O:" + ordinaryUser + "D:PAI(A;;FA;;;" + ordinaryUser + ")(A;;FA;;;SY)(A;;FA;;;BA)", ""},
		{"ordinary user plus another user", ordinaryUser, "O:" + ordinaryUser + "D:(A;;FA;;;" + ordinaryUser + ")(A;;FR;;;" + otherUser + ")", "non-owner principal"},
		{"ordinary user plus Everyone", ordinaryUser, "O:" + ordinaryUser + "D:(A;;FA;;;" + ordinaryUser + ")(A;;FR;;;WD)", "broad group"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := evalCredsSecurityResolved(c.sddl, c.me, resolve)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("evalCredsSecurityResolved(%q, %s) refused: %v", c.sddl, c.me, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("evalCredsSecurityResolved(%q, %s) err=%v, want a refusal containing %q", c.sddl, c.me, err, c.wantErr)
			}
		})
	}
}

// A resolver that answers with something other than a SID, or an empty SID,
// must not turn an alias into a match, even for an empty current-user SID.
func TestEvalCredsSecurityAliasResolverFailsClosed(t *testing.T) {
	bogus := func(string) (string, bool) { return "", true }
	notASID := func(string) (string, bool) { return "LA", true }
	for _, me := range []string{"S-1-5-21-1-2-3-500", ""} {
		for name, r := range map[string]sddlAliasResolver{"nil": nil, "empty": bogus, "not-a-sid": notASID} {
			if err := evalCredsSecurityResolved("O:LAD:(A;;FA;;;LA)", me, r); err == nil {
				t.Errorf("resolver %s, me=%q: unresolved LA was accepted", name, me)
			}
		}
	}
	// Without a resolver, the evaluator keeps its machine-independent verdicts.
	if err := evalCredsSecurity("O:BAD:(A;;FA;;;SY)(A;;FA;;;BA)", "S-1-5-21-1-2-3-1000"); err != nil {
		t.Errorf("BA owner with SY/BA ACEs refused without a resolver: %v", err)
	}
}

// Windows renders a real NULL DACL with its flags first: measured on Windows
// Server 2025 as "O:LAD:PAINO_ACCESS_CONTROL". Every form is world-accessible.
func TestEvalCredsSecurityRefusesNullDACLBehindFlags(t *testing.T) {
	const me = "S-1-5-21-1-2-3-1000"
	for _, dacl := range []string{"NO_ACCESS_CONTROL", "PNO_ACCESS_CONTROL", "AINO_ACCESS_CONTROL", "PAINO_ACCESS_CONTROL", "PAIARNO_ACCESS_CONTROL"} {
		sddl := "O:" + me + "D:" + dacl
		if err := evalCredsSecurity(sddl, me); err == nil || !strings.Contains(err.Error(), "NULL DACL") {
			t.Errorf("evalCredsSecurity(%q) err=%v, want a NULL DACL refusal", sddl, err)
		}
	}
	builtinAdmin := func(alias string) (string, bool) {
		if alias == "LA" {
			return "S-1-5-21-1-2-3-500", true
		}
		return "", false
	}
	if err := evalCredsSecurityResolved("O:LAD:PAINO_ACCESS_CONTROL", "S-1-5-21-1-2-3-500", builtinAdmin); err == nil || !strings.Contains(err.Error(), "NULL DACL") {
		t.Errorf("measured NULL DACL shape err=%v, want a NULL DACL refusal", err)
	}
	// Looking at the flags prefix does not refuse an ordinary protected DACL.
	if err := evalCredsSecurity("O:"+me+"D:PAI(A;;FA;;;"+me+")", me); err != nil {
		t.Errorf("protected owner-only DACL refused: %v", err)
	}
}

// The DACL section is split exactly: flags, then complete ACEs, then an
// optional SACL section that grants nothing. Anything else is refused.
func TestEvalCredsSecurityParsesTheDACLSectionExactly(t *testing.T) {
	const me = "S-1-5-21-1-2-3-1000"
	own := "(A;;FA;;;" + me + ")"
	cases := []struct {
		name    string
		sddl    string
		wantErr string
	}{
		{"empty DACL, NULL SACL", "O:BAD:S:NO_ACCESS_CONTROL", ""},
		{"empty protected DACL, audit SACL", "O:BAD:PS:(AU;SA;FA;;;WD)", ""},
		{"owner-only DACL, audit SACL for Everyone", "O:" + me + "D:PAI" + own + "S:AI(AU;SA;FA;;;WD)", ""},
		{"NULL DACL followed by a SACL", "O:" + me + "D:PAINO_ACCESS_CONTROLS:NO_ACCESS_CONTROL", "NULL DACL"},
		{"unknown DACL flag", "O:" + me + "D:XY" + own, "unparseable flags"},
		{"lowercase NULL DACL flags", "O:" + me + "D:paino_access_control", "unparseable flags"},
		{"unclosed trailing ACE", "O:" + me + "D:PAI" + own + "(A;;FA;;;WD", "unparseable ACE"},
		{"text between ACEs", "O:" + me + "D:PAI" + own + "junk", "unparseable ACE"},
		{"conditional ACE", "O:" + me + "D:PAI" + own + "(XA;;FA;;;WD;(Member_of{SID(BA)}))", "unparseable ACE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := evalCredsSecurity(c.sddl, me)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("evalCredsSecurity(%q) refused: %v", c.sddl, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("evalCredsSecurity(%q) err=%v, want a refusal containing %q", c.sddl, err, c.wantErr)
			}
		})
	}
}
