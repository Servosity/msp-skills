// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package coverpc

import (
	"encoding/json"
	"testing"
)

// jsonEscape is the six-character JSON escape for the code unit hex, built at
// run time so no tool or editor decodes it on the way into this file.
func jsonEscape(hex string) string { return string(rune(92)) + "u" + hex }

// The one read rule, judged on the decoded method name Cove dispatches on.
func TestIsReadMethodJudgesTheDecodedName(t *testing.T) {
	for _, m := range []string{"GetServerInfo", "GetAccountInfoById", "EnumerateAccounts", "EnumerateAccountStatistics", "GetX", "GetX2"} {
		if !IsReadMethod(m) {
			t.Errorf("IsReadMethod(%q) = false, want true", m)
		}
	}
	writes := []string{
		"", "Get", "Enumerate", "Getaway", "Enumerated", "GETRemoveAccount", "getAccountInfoById",
		"enumerateAccounts", "RemoveAccount", "ModifyAccount", "AddPartner", "SetAccountProfile",
		" GetServerInfo", "GetServerInfo ", "Get ServerInfo", "GetServerInfo" + string(rune(0)) + "RemoveAccount",
		"GetServerInfo\nRemoveAccount", "Get_ServerInfo", "GetServer.Info", "GetÄccount", "GetServerİnfo",
		"Get" + string(rune(0x200b)) + "ServerInfo", string(rune(0xfeff)) + "GetServerInfo",
		string([]rune{0xff27, 0xff45, 0xff54}) + "ServerInfo", "GetServerInfo" + string(rune(0xfffd)),
	}
	for _, m := range writes {
		if IsReadMethod(m) {
			t.Errorf("IsReadMethod(%q) = true, want false", m)
		}
	}
	// Escapes are judged after JSON decoding: 0047 is G, 0052 is R.
	doc := `["` + jsonEscape("0047") + `etServerInfo","` + jsonEscape("0052") + `emoveAccount","Get` + jsonEscape("0000") + `X"]`
	var decoded []string
	if err := json.Unmarshal([]byte(doc), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0] != "GetServerInfo" || !IsReadMethod(decoded[0]) || IsReadMethod(decoded[1]) || IsReadMethod(decoded[2]) {
		t.Fatalf("decoded escapes judged wrongly: %q", decoded)
	}
}

func TestEveryEndpointMethodIsARead(t *testing.T) {
	ids := EndpointIDs()
	if len(ids) < 16 {
		t.Fatalf("endpoint table has %d entries, want at least 16", len(ids))
	}
	for _, id := range ids {
		m, ok := EndpointMethod(id)
		if !ok || !IsReadMethod(m) {
			t.Errorf("%s -> %q is not a read", id, m)
		}
	}
	if m, ok := EndpointMethodForTool("backup-jobs_list"); !ok || m != "EnumerateJobs" {
		t.Errorf("EndpointMethodForTool(backup-jobs_list) = %q, %v", m, ok)
	}
	if _, ok := EndpointMethodForTool("call"); ok {
		t.Error("EndpointMethodForTool(call) names a method")
	}
}
