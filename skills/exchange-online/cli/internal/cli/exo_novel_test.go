// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil/testenv"
)

const testTenant = "0550b473-9fd2-4dbb-a058-a1640b4bf458"

type fakeEXO struct {
	mu    sync.Mutex
	calls []string // "Cmdlet <json params>"
	reply func(cmdlet string, p map[string]any) any
}

func newFakeEXO(t *testing.T, reply func(cmdlet string, p map[string]any) any) *fakeEXO {
	t.Helper()
	f := &fakeEXO{reply: reply}
	// The generated client only speaks HTTP/2, so the fake needs TLS + h2 (and --insecure).
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env struct {
			CmdletInput struct {
				CmdletName string         `json:"CmdletName"`
				Parameters map[string]any `json:"Parameters"`
			} `json:"CmdletInput"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &env)
		in := env.CmdletInput
		p, _ := json.Marshal(in.Parameters)
		f.mu.Lock()
		f.calls = append(f.calls, in.CmdletName+" "+string(p))
		f.mu.Unlock()
		rows := f.reply(in.CmdletName, in.Parameters)
		if e, ok := rows.(error); ok { // an Exchange cmdlet error
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "BadRequest", "message": e.Error()}})
			return
		}
		if rows == nil {
			rows = []any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": rows})
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("EXCHANGE_ONLINE_BASE_URL", srv.URL)
	t.Setenv("EXCHANGE_ONLINE_TOKEN", "test-token")
	return f
}

func (f *fakeEXO) called(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func runEXO(t *testing.T, args ...string) map[string]any {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append(args, "--json", "--insecure"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstderr: %s\nstdout: %s", args, err, errb.String(), out.String())
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("%v: output is not a JSON object: %v\n%s", args, err, out.String())
	}
	if res, ok := v["results"].(map[string]any); ok { // generated --json envelope
		return res
	}
	return v
}

func domains(string, map[string]any) any {
	return []any{map[string]any{"DomainName": "example.com"}}
}

func TestEwsPostureResolvesInheritance(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-OrganizationConfig":
			return []any{map[string]any{"EwsEnabled": false}}
		case "Get-TransportConfig":
			return []any{map[string]any{"SmtpClientAuthenticationDisabled": true}}
		case "Get-CASMailbox":
			return []any{
				map[string]any{"PrimarySmtpAddress": "a@example.com", "EwsEnabled": true, "SmtpClientAuthenticationDisabled": nil},
				map[string]any{"PrimarySmtpAddress": "b@example.com", "EwsEnabled": true, "SmtpClientAuthenticationDisabled": false, "PopEnabled": false},
			}
		}
		return nil
	})
	v := runEXO(t, "ews-posture", testTenant, "--still-enabled", "smtpauth")
	counts := v["enabled_counts"].(map[string]any)
	if counts["ews"].(float64) != 0 || counts["smtpauth"].(float64) != 1 || counts["pop"].(float64) != 1 {
		t.Fatalf("counts = %v", counts)
	}
	list := v["list"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["mailbox"] != "b@example.com" {
		t.Fatalf("still-enabled list = %v", list)
	}
}

func TestForwardingAuditVerdicts(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-HostedOutboundSpamFilterPolicy":
			return []any{map[string]any{"Name": "Default", "IsDefault": true, "AutoForwardingMode": "On"}}
		case "Get-RemoteDomain":
			return []any{map[string]any{"DomainName": "*", "AutoForwardEnabled": true}, map[string]any{"DomainName": "blocked.example", "AutoForwardEnabled": false}}
		case "Get-Mailbox":
			return []any{
				map[string]any{"UserPrincipalName": "a@example.com", "PrimarySmtpAddress": "a@example.com", "RecipientTypeDetails": "UserMailbox", "ForwardingSmtpAddress": "smtp:x@gmail.example"},
				map[string]any{"UserPrincipalName": "b@example.com", "PrimarySmtpAddress": "b@example.com", "RecipientTypeDetails": "UserMailbox", "ForwardingSmtpAddress": "smtp:y@blocked.example"},
			}
		case "Get-InboxRule":
			if p["Mailbox"] == "a@example.com" {
				return []any{map[string]any{"Name": "x", "Identity": "a\\1", "Enabled": true, "RedirectTo": []any{"\"Z\" [SMTP:z@evil.example]"}}}
			}
		case "Get-TransportRule":
			return []any{map[string]any{"Name": "bcc", "State": "Enabled", "BlindCopyTo": []any{"archive@vendor.example"}},
				map[string]any{"Name": "audit", "State": "Enabled", "Mode": "Audit", "RedirectMessageTo": []any{"audit@vendor.example"}}}
		}
		return nil
	})
	v := runEXO(t, "forwarding-audit", testTenant, "--sweep")
	byTarget := map[string]string{}
	for _, p := range v["paths"].([]any) {
		m := p.(map[string]any)
		byTarget[m["target"].(string)] = m["verdict"].(string)
	}
	want := map[string]string{"x@gmail.example": "leaves-tenant", "y@blocked.example": "blocked-by-remote-domain",
		"z@evil.example": "leaves-tenant", "archive@vendor.example": "leaves-tenant", "audit@vendor.example": "rule-not-enforced"}
	for k, w := range want {
		if byTarget[k] != w {
			t.Fatalf("verdict[%s] = %q, want %q (all: %v)", k, byTarget[k], w, byTarget)
		}
	}
	// The stored sweep is reused without --sweep.
	v = runEXO(t, "forwarding-audit", testTenant)
	if v["inbox_rules"].(map[string]any)["source"] != "stored-sweep" {
		t.Fatalf("inbox_rules = %v", v["inbox_rules"])
	}
}

// A reason prints once on every branch of the verdict, and a resolved target adds to the
// rule's reason rather than replacing it.
func TestForwardingReasonPrintsOnce(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-HostedOutboundSpamFilterPolicy":
			return []any{map[string]any{"IsDefault": true, "AutoForwardingMode": "On"}}
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "a@example.com", "PrimarySmtpAddress": "a@example.com", "RecipientTypeDetails": "UserMailbox", "ForwardingSmtpAddress": "smtp:gated@vendor.example"}}
		case "Get-TransportRule":
			return []any{
				map[string]any{"Name": "bcc", "State": "Enabled", "BlindCopyTo": []any{"archive@vendor.example", "arch@example.com", "inside@example.com"}},
				map[string]any{"Name": "audit", "State": "Enabled", "Mode": "Audit", "RedirectMessageTo": []any{"audit@vendor.example"}},
				map[string]any{"Name": "off", "State": "Disabled", "CopyTo": []any{"off@vendor.example"}},
			}
		case "Get-Recipient":
			if p["Identity"] == "arch@example.com" { // an internal-addressed contact pointing outside
				return []any{map[string]any{"PrimarySmtpAddress": "arch@example.com", "ExternalEmailAddress": "SMTP:arch@outside.example"}}
			}
			if p["Identity"] == "inside@example.com" {
				return []any{map[string]any{"PrimarySmtpAddress": "inside@example.com"}}
			}
		}
		return nil
	})
	v := runEXO(t, "forwarding-audit", testTenant, "--all")
	reasons := map[string]string{}
	for _, p := range v["paths"].([]any) {
		m := p.(map[string]any)
		reasons[m["target"].(string)] = m["reason"].(string)
	}
	const enforced = "enabled mail flow rule in Enforce mode; not subject to automatic-forwarding controls"
	for target, want := range map[string]string{
		"archive@vendor.example": enforced,
		"arch@outside.example":   "resolves to arch@outside.example; " + enforced,
		"inside@example.com":     "target is in an accepted domain",
		"audit@vendor.example":   "mail flow rule Mode is Audit: actions are only logged",
		"off@vendor.example":     "mail flow rule State is Disabled",
		"gated@vendor.example":   "outbound policy On and remote domain (none) allows auto-forwarding",
	} {
		if reasons[target] != want {
			t.Errorf("reason[%s] = %q, want %q", target, reasons[target], want)
		}
	}
}

func TestBecTriageSignals(t *testing.T) {
	recent := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05")
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "alex@tenant.onmicrosoft.example", "PrimarySmtpAddress": "alex@example.com",
				"EmailAddresses": []any{"SMTP:alex@example.com", "smtp:alex@tenant.onmicrosoft.example"}, "ForwardingSmtpAddress": "smtp:arch@example.com"}}
		case "Get-Recipient": // an internal-addressed contact that delivers outside
			return []any{map[string]any{"PrimarySmtpAddress": "arch@example.com", "ExternalEmailAddress": "SMTP:arch@outside.example"}}
		case "Get-MessageTraceV2":
			if got := fmt.Sprint(p["SenderAddress"]); got != "[alex@example.com alex@tenant.onmicrosoft.example]" {
				t.Errorf("trace queried senders %v, want every SMTP address of the mailbox", got)
			}
		case "Get-InboxRule":
			if p["IncludeHidden"] != true {
				t.Errorf("Get-InboxRule without IncludeHidden")
			}
			return []any{map[string]any{"Name": ".", "Enabled": true, "DeleteMessage": true}}
		case "Get-BlockedSenderAddress": // keyed by UPN
			return []any{map[string]any{"SenderAddress": "alex@tenant.onmicrosoft.example", "Reason": "OutboundSpamLast24Hours"}}
		case "Get-MobileDevice":
			return []any{map[string]any{"FriendlyName": "iPhone", "FirstSyncTime": recent}}
		}
		return nil
	})
	v := runEXO(t, "bec", "triage", testTenant, "alex@tenant.onmicrosoft.example")
	if v["severity"] != "high" {
		t.Fatalf("severity = %v", v["severity"])
	}
	got := map[string]bool{}
	for _, s := range v["signals"].([]any) {
		got[s.(map[string]any)["signal"].(string)] = true
	}
	for _, w := range []string{"suspicious-inbox-rule", "restricted-sender", "new-mobile-device", "external-mailbox-forwarding"} {
		if !got[w] {
			t.Fatalf("missing signal %s in %v", w, got)
		}
	}
}

func TestOffboardReportsUnconfirmedChanges(t *testing.T) {
	shared := false
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Set-Mailbox":
			if p["Type"] == "Shared" {
				shared = true
			}
		case "Get-Mailbox":
			t := "UserMailbox"
			if shared {
				t = "SharedMailbox"
			}
			return []any{map[string]any{"UserPrincipalName": "jane@example.com", "RecipientTypeDetails": t, "HiddenFromAddressListsEnabled": false}}
		case "Get-Recipient":
			return []any{map[string]any{"PrimarySmtpAddress": "boss@example.com", "Guid": "b055", "RecipientTypeDetails": "UserMailbox"}}
		case "Get-MailboxPermission":
			// FullAccess allowed but also denied: not effective.
			return []any{map[string]any{"User": "Boss Person", "AccessRights": []any{"FullAccess"}},
				map[string]any{"User": "Boss Person", "AccessRights": []any{"FullAccess"}, "Deny": true}}
		case "Get-RecipientPermission": // returned under the canonical name, filtered by Exchange
			return []any{map[string]any{"Trustee": "boss@corp.example", "AccessRights": []any{"SendAs"}, "AccessControlType": "Allow"}}
		}
		return nil
	})
	v := runEXO(t, "offboard", testTenant, "jane@example.com", "--manager", "boss@example.com", "--keep-devices")
	status := map[string]string{}
	for _, s := range v["steps"].([]any) {
		m := s.(map[string]any)
		status[m["step"].(string)] = m["status"].(string)
	}
	// The fake confirms the shared conversion but never shows the GAL hide or the grants.
	want := map[string]string{"convert-to-shared": "done", "hide-from-address-list": "unverified", "manager-full-access": "unverified", "manager-send-as": "already-done"}
	for k, w := range want {
		if status[k] != w {
			t.Fatalf("step %s = %q, want %q (all %v)", k, status[k], w, status)
		}
	}
}

func TestOffboardPlanThenApply(t *testing.T) {
	f := newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "jane@example.com", "RecipientTypeDetails": "UserMailbox", "HiddenFromAddressListsEnabled": false}}
		case "Get-MobileDevice":
			return []any{map[string]any{"Identity": "dev1", "FriendlyName": "Pixel"}}
		case "Get-Recipient":
			return []any{map[string]any{"PrimarySmtpAddress": "boss@example.com", "RecipientTypeDetails": "UserMailbox"}}
		}
		return nil
	})
	v := runEXO(t, "offboard", testTenant, "jane@example.com", "--manager", "boss@example.com", "--plan")
	for _, s := range v["steps"].([]any) {
		if st := s.(map[string]any)["status"]; st != "pending" {
			t.Fatalf("plan step %v status %v", s, st)
		}
	}
	if n := len(f.called("Set-")) + len(f.called("Add-")) + len(f.called("Remove-")); n != 0 {
		t.Fatalf("--plan made %d changes", n)
	}
	runEXO(t, "offboard", testTenant, "jane@example.com", "--manager", "boss@example.com")
	for _, w := range []string{`Set-Mailbox {"Identity":"jane@example.com","Type":"Shared"}`, `Remove-MobileDevice {"Identity":"dev1"}`} {
		found := false
		for _, c := range f.calls {
			found = found || c == w
		}
		if !found {
			t.Fatalf("missing call %s in %v", w, f.calls)
		}
	}
	if len(f.called("Add-MailboxPermission")) != 1 || len(f.called("Add-RecipientPermission")) != 1 {
		t.Fatalf("manager grants not sent: %v", f.calls)
	}
}

func TestQuarantineQueueSpansTenantTokens(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		return []any{map[string]any{"Identity": "q", "ReceivedTime": "2026-10-14T10:00:00", "SenderAddress": "x@bad.example"}}
	})
	runEXO(t, "quarantine", "snapshot", testTenant)
	t.Setenv("EXCHANGE_ONLINE_TOKEN", "another-tenants-token") // each tenant has its own token
	runEXO(t, "quarantine", "snapshot", "Contoso.OnMicrosoft.Example")
	runEXO(t, "quarantine", "snapshot", "contoso.onmicrosoft.example") // same tenant, other spelling
	v := runEXO(t, "quarantine", "queue")
	if n := len(v["snapshots"].([]any)); n != 2 {
		t.Fatalf("queue sees %d tenant snapshots, want 2: %v", n, v["snapshots"])
	}
}

func TestQuarantineEmptySnapshotStaysVisible(t *testing.T) {
	newFakeEXO(t, func(string, map[string]any) any { return nil })
	runEXO(t, "quarantine", "snapshot", testTenant)
	v := runEXO(t, "quarantine", "queue", "--tenant", testTenant)
	snaps := v["snapshots"].([]any)
	if len(snaps) != 1 || snaps[0].(map[string]any)["requests"].(float64) != 0 || v["note"] != nil {
		t.Fatalf("snapshots = %v note = %v", snaps, v["note"])
	}
}

func TestQuarantineSnapshotAndQueue(t *testing.T) {
	old := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02T15:04:05")
	newFakeEXO(t, func(c string, p map[string]any) any {
		if c == "Get-QuarantineMessage" && p["ReleaseStatus"] == "Requested" {
			return []any{map[string]any{"Identity": "q1", "ReceivedTime": old, "SenderAddress": "bad@phish.example", "Type": "High Confidence Phish", "RecipientAddress": []any{"a@example.com"}}}
		}
		return nil
	})
	runEXO(t, "quarantine", "snapshot", testTenant)
	v := runEXO(t, "quarantine", "queue", "--older-than", "24h")
	q := v["queue"].([]any)
	if len(q) != 1 || q[0].(map[string]any)["admin_only"] != true || q[0].(map[string]any)["tenant"] != testTenant {
		t.Fatalf("queue = %v", q)
	}
	if v = runEXO(t, "quarantine", "queue", "--older-than", "72h"); len(v["queue"].([]any)) != 0 || v["hidden_by_older_than"].(float64) != 1 {
		t.Fatalf("--older-than 72h kept %v (hidden %v)", v["queue"], v["hidden_by_older_than"])
	}
}

func TestWhereIsFollowsTraceContinuation(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		if c != "Get-MessageTraceV2" {
			return nil
		}
		if p["StartingRecipientAddress"] == nil { // first page: a full page of other mail
			rows := make([]any, 0, 1000)
			for i := 0; i < 1000; i++ {
				rows = append(rows, map[string]any{"Received": fmt.Sprintf("2026-10-14T10:%02d:%02d.9", (i/60)%60, i%60), "RecipientAddress": "a@example.com",
					"MessageTraceId": fmt.Sprint("m", i), "Subject": "newsletter", "Status": "Delivered"})
			}
			return rows
		}
		if p["EndDate"] != "2026-10-14T10:16:39.9" {
			t.Errorf("continuation EndDate %v lost the last row's precision", p["EndDate"])
		}
		return []any{map[string]any{"Received": "2026-10-14T09:00:00", "RecipientAddress": "a@example.com", "MessageTraceId": "old",
			"Subject": "Invoice 7", "Status": "FilteredAsSpam"}}
	})
	v := runEXO(t, "where-is", testTenant, "--recipient", "a@example.com", "--subject", "invoice")
	msgs := v["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["verdict"] != "delivered-to-junk" || v["trace_complete"] != true {
		t.Fatalf("continuation not followed: %v", v)
	}
}

func TestForwardingPrecedenceAndAccessMapIdentity(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-HostedOutboundSpamFilterPolicy":
			return []any{map[string]any{"IsDefault": true, "AutoForwardingMode": "On"}}
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "a@example.com", "PrimarySmtpAddress": "a@example.com",
				"ForwardingAddress": "Boss", "ForwardingSmtpAddress": "smtp:x@outside.example", "GrantSendOnBehalfTo": []any{"Jane Doe"}}}
		case "Get-Recipient":
			if p["Filter"] != nil { // display-name uniqueness check: two Jane Does
				return []any{map[string]any{"Identity": "1"}, map[string]any{"Identity": "2"}}
			}
			if p["Identity"] == "Boss" {
				return []any{map[string]any{"Name": "Boss", "PrimarySmtpAddress": "boss@example.com"}}
			}
			if p["Identity"] == "Jane Doe" {
				return fmt.Errorf("The operation couldn't be performed because 'Jane Doe' matches multiple entries.")
			}
			return []any{map[string]any{"Name": "jdoe", "DisplayName": "Jane Doe", "PrimarySmtpAddress": "jane@example.com"}}
		}
		return nil
	})
	v := runEXO(t, "forwarding-audit", testTenant, "--all")
	got := map[string]string{}
	for _, p := range v["paths"].([]any) {
		m := p.(map[string]any)
		got[m["target"].(string)] = m["verdict"].(string)
	}
	if got["x@outside.example"] != "inactive" || got["boss@example.com"] != "internal" {
		t.Fatalf("forwarding precedence: %v", got)
	}
	v = runEXO(t, "access-map", testTenant, "jane@example.com", "--skip-full-access")
	if len(v["access"].([]any)) != 0 || len(v["ambiguous"].([]any)) != 1 {
		t.Fatalf("display-name-only grant should be ambiguous: access=%v ambiguous=%v", v["access"], v["ambiguous"])
	}
}

func TestWhereIsQuarantined(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-MessageTraceV2":
			return []any{map[string]any{"Received": "2026-10-14T10:00:00", "SenderAddress": "x@vendor.example", "RecipientAddress": "a@example.com",
				"Subject": "Invoice 42", "Status": "Quarantined", "MessageId": "<m1@vendor.example>"}}
		case "Get-QuarantineMessage":
			if p["SenderAddress"] != "x@vendor.example" || p["StartReceivedDate"] == nil {
				t.Errorf("quarantine lookup not scoped: %v", p)
			}
			return []any{map[string]any{"Identity": "q1", "Type": "Spam", "ReleaseStatus": "NotReleased", "SenderAddress": "x@vendor.example", "RecipientAddress": []any{"a@example.com"}},
				map[string]any{"Identity": "q2", "SenderAddress": "", "RecipientAddress": []any{"a@example.com"}}}
		}
		return nil
	})
	v := runEXO(t, "where-is", testTenant, "--recipient", "a@example.com", "--subject", "invoice")
	m := v["messages"].([]any)[0].(map[string]any)
	if m["verdict"] != "quarantined" || !strings.Contains(m["next_action"].(string), "--identity 'q1' --user a@example.com") {
		t.Fatalf("message = %v", m)
	}
}

func TestAccessMapFindsGrants(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-Recipient":
			return []any{map[string]any{"Name": "Jane Doe", "Alias": "jane", "PrimarySmtpAddress": "jane@example.com"}}
		case "Get-Mailbox":
			return []any{
				map[string]any{"Name": "Sales", "PrimarySmtpAddress": "sales@example.com", "GrantSendOnBehalfTo": []any{"Jane Doe"}},
				map[string]any{"Name": "Finance", "PrimarySmtpAddress": "finance@example.com"},
			}
		case "Get-RecipientPermission":
			return []any{map[string]any{"Identity": "Finance", "Trustee": "jane@example.com", "AccessRights": []any{"SendAs"}, "AccessControlType": "Allow"}}
		case "Get-MailboxPermission":
			if p["Identity"] == "finance@example.com" {
				return []any{map[string]any{"User": "jane@example.com", "AccessRights": []any{"FullAccess"}, "IsInherited": false}}
			}
		}
		return nil
	})
	v := runEXO(t, "access-map", testTenant, "jane@example.com")
	got := map[string]string{}
	for _, e := range v["access"].([]any) {
		m := e.(map[string]any)
		var parts []string
		for _, a := range m["access"].([]any) {
			parts = append(parts, a.(string))
		}
		got[m["mailbox"].(string)] = strings.Join(parts, ",")
	}
	if got["sales@example.com"] != "SendOnBehalf" || got["finance@example.com"] != "FullAccess,SendAs" {
		t.Fatalf("access = %v", got)
	}
}

func TestForwardingResolvesContactsAndSweepScope(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-HostedOutboundSpamFilterPolicy":
			return []any{map[string]any{"IsDefault": true, "AutoForwardingMode": "On"}}
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "a@example.com", "PrimarySmtpAddress": "a@example.com", "RecipientTypeDetails": "SharedMailbox"}}
		case "Get-InboxRule":
			return []any{map[string]any{"Name": "r", "Enabled": true, "RedirectTo": []any{`"Arch" [SMTP:arch@example.com]`, `"Lex" [EX:/o=ExchangeLabs/cn=lex]`}}}
		case "Get-Recipient":
			if p["Identity"] == "arch@example.com" { // an internal-addressed contact pointing outside
				return []any{map[string]any{"PrimarySmtpAddress": "arch@example.com", "ExternalEmailAddress": "SMTP:arch@outside.example"}}
			}
			if strings.Contains(fmt.Sprint(p["Filter"]), "LegacyExchangeDN") {
				return []any{map[string]any{"PrimarySmtpAddress": "lex@example.com", "LegacyExchangeDN": "/o=ExchangeLabs/cn=lex"}}
			}
		}
		return nil
	})
	runEXO(t, "inbox-rules", "sweep", testTenant, "--mailbox-types", "SharedMailbox")
	v := runEXO(t, "forwarding-audit", testTenant, "--all")
	got := map[string]string{}
	for _, p := range v["paths"].([]any) {
		m := p.(map[string]any)
		got[m["target"].(string)] = m["verdict"].(string)
	}
	if got["arch@outside.example"] != "leaves-tenant" || got["lex@example.com"] != "internal" {
		t.Fatalf("resolved targets = %v", got)
	}
	if v["inbox_rules"].(map[string]any)["incomplete"] != true {
		t.Fatalf("a SharedMailbox-only sweep must be marked incomplete: %v", v["inbox_rules"])
	}
}

func TestOffboardRefusesExternalForwarding(t *testing.T) {
	f := newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-AcceptedDomain":
			return domains(c, p)
		case "Get-Mailbox": // nothing done yet: every step would write
			return []any{map[string]any{"UserPrincipalName": "jane@example.com", "RecipientTypeDetails": "UserMailbox", "HiddenFromAddressListsEnabled": false}}
		case "Get-MobileDevice":
			return []any{map[string]any{"Identity": "dev1"}}
		case "Get-Recipient":
			return []any{map[string]any{"PrimarySmtpAddress": "boss@example.com", "ExternalEmailAddress": "SMTP:boss@home.example"}}
		}
		return nil
	})
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"offboard", testTenant, "jane@example.com", "--forward-to", "boss@example.com", "--json", "--insecure"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error()+out.String(), "only forwards internally") {
		t.Fatalf("external forwarding not refused: err=%v out=%s", err, out.String())
	}
	if n := len(f.called("Set-")) + len(f.called("Add-")) + len(f.called("Remove-")); n != 0 {
		t.Fatalf("writes sent before validation failed: %v", f.calls)
	}
}

func TestBecBurstCountsTraceIDsAndReplyCompare(t *testing.T) {
	base := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	var trace []exoRow
	for i := 0; i < 10; i++ { // ten messages reusing one sender-set Message-ID
		trace = append(trace, exoRow{"Received": base.Add(time.Duration(i) * time.Second).Format(time.RFC3339), "MessageId": "<same@x>",
			"MessageTraceId": fmt.Sprint("t", i), "RecipientAddress": "r@outside.example", "Subject": fmt.Sprint("s", i)})
	}
	var v becView
	sig := outboundSignals(trace, map[string]bool{"example.com": true}, &v)
	if len(sig) == 0 || sig[0].Signal != "send-burst" {
		t.Fatalf("burst not detected: %v", sig)
	}
	a := normalizeReply(`<html><body>Contact <a href="mailto:old@example.com">the manager</a></body></html>`)
	b := normalizeReply(`Contact <a href="mailto:new@example.com">the manager</a>`)
	if a == b || normalizeReply("<p>Hi&nbsp;there</p>") != normalizeReply("Hi there") {
		t.Fatalf("reply compare: %q vs %q", a, b)
	}
}

func TestOffboardRejectsContactManager(t *testing.T) {
	f := newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "jane@example.com", "RecipientTypeDetails": "UserMailbox"}}
		case "Get-Recipient":
			return []any{map[string]any{"PrimarySmtpAddress": "boss@example.com", "RecipientTypeDetails": "MailContact"}}
		}
		return nil
	})
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"offboard", testTenant, "jane@example.com", "--manager", "boss@example.com", "--json", "--insecure"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error()+out.String(), "MailContact") {
		t.Fatalf("contact manager accepted: %v %s", err, out.String())
	}
	if n := len(f.called("Set-")) + len(f.called("Add-")) + len(f.called("Remove-")); n != 0 {
		t.Fatalf("writes sent: %v", f.calls)
	}
}

func TestRecipientMustAnswerToItsIdentity(t *testing.T) {
	// Exchange can answer an unmatched identity with some other recipient.
	newFakeEXO(t, func(c string, p map[string]any) any {
		if c == "Get-Recipient" {
			return []any{map[string]any{"Name": "Someone", "PrimarySmtpAddress": "someone@example.com", "RecipientTypeDetails": "UserMailbox"}}
		}
		return nil
	})
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"access-map", testTenant, "typo@example.com", "--json", "--insecure"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error()+out.String(), "not found") {
		t.Fatalf("unrelated recipient accepted for a typo: %v %s", err, out.String())
	}
}

func TestRecipientIdentityChecksDNsAndUPNs(t *testing.T) {
	r := recipientOf(exoRow{"PrimarySmtpAddress": "a@example.com", "DistinguishedName": "CN=alice@example.com,OU=Users,DC=example,DC=com"})
	if !r.answers("CN=alice@example.com,OU=Users,DC=example,DC=com") || !r.answers("A@example.com") || r.answers("b@example.com") {
		t.Fatal("identity check mismatch")
	}
	// A mailbox found by a UPN that is not one of its SMTP proxies.
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-Recipient":
			return []any{map[string]any{"PrimarySmtpAddress": "jo@example.com", "RecipientTypeDetails": "UserMailbox"}}
		case "Get-Mailbox":
			return []any{map[string]any{"UserPrincipalName": "jo@tenant.onmicrosoft.example", "PrimarySmtpAddress": "jo@example.com"}}
		}
		return nil
	})
	d := newDestResolver(context.Background(), mustClient(t), testTenant, map[string]bool{"example.com": true, "tenant.onmicrosoft.example": true}).resolve("jo@tenant.onmicrosoft.example")
	if d.Addr != "jo@example.com" || d.Unresolved != "" {
		t.Fatalf("UPN destination = %+v", d)
	}
}

func mustClient(t *testing.T) *client.Client {
	t.Helper()
	var flags rootFlags
	flags.insecure = true
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAccessMapCalendarDefaultAndLocalizedFolder(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		switch c {
		case "Get-Recipient":
			return []any{map[string]any{"Name": "jdoe", "DisplayName": "Jane Doe", "PrimarySmtpAddress": "jane@example.com"}}
		case "Get-Mailbox":
			if p["Identity"] != nil {
				return nil
			}
			return []any{map[string]any{"PrimarySmtpAddress": "one@example.com"}, map[string]any{"PrimarySmtpAddress": "two@example.com"}}
		case "Get-MailboxFolderStatistics":
			return []any{map[string]any{"Name": "Kalender", "FolderType": "Calendar"}}
		case "Get-MailboxFolderPermission":
			if !strings.HasSuffix(fmt.Sprint(p["Identity"]), `:\Kalender`) {
				t.Errorf("calendar folder not discovered: %v", p["Identity"])
			}
			rows := []any{map[string]any{"User": "Default", "AccessRights": []any{"Editor"}}}
			if p["Identity"] == `two@example.com:\Kalender` { // explicit None overrides Default
				rows = append(rows, map[string]any{"User": "jane@example.com", "AccessRights": []any{"None"}})
			}
			return rows
		}
		return nil
	})
	v := runEXO(t, "access-map", testTenant, "jane@example.com", "--calendars", "--skip-full-access")
	got := map[string]string{}
	for _, e := range v["access"].([]any) {
		m := e.(map[string]any)
		got[m["mailbox"].(string)] = fmt.Sprint(m["access"])
	}
	if got["one@example.com"] != "[Calendar:Editor (Default)]" || got["two@example.com"] != "" {
		t.Fatalf("calendar access = %v", got)
	}
}

func TestGroupsAreNotTreatedAsInternalDestinations(t *testing.T) {
	newFakeEXO(t, func(c string, p map[string]any) any {
		if c == "Get-Recipient" {
			return []any{map[string]any{"PrimarySmtpAddress": "archive@example.com", "RecipientTypeDetails": "MailUniversalDistributionGroup"}}
		}
		return nil
	})
	d := newDestResolver(context.Background(), mustClient(t), testTenant, map[string]bool{"example.com": true}).resolve("archive@example.com")
	if d.Unresolved == "" || !strings.Contains(d.Unresolved, "group") {
		t.Fatalf("group destination = %+v", d)
	}
}

func TestSubdomainsAndEXGroups(t *testing.T) {
	acc := map[string]bool{"contoso.example": true, "*.contoso.example": true, "fabrikam.example": true}
	if isExternal("u@branch.contoso.example", acc) || !isExternal("u@branch.fabrikam.example", acc) || isExternal("u@contoso.example", acc) {
		t.Fatal("MatchSubDomains handling")
	}
	newFakeEXO(t, func(c string, p map[string]any) any {
		if c == "Get-Recipient" {
			return []any{map[string]any{"PrimarySmtpAddress": "grp@contoso.example", "LegacyExchangeDN": "/o=x/cn=grp", "RecipientTypeDetails": "MailUniversalDistributionGroup"}}
		}
		return nil
	})
	d := newDestResolver(context.Background(), mustClient(t), testTenant, acc).resolve(`"Group" [EX:/o=x/cn=grp]`)
	if !strings.Contains(d.Unresolved, "group") {
		t.Fatalf("EX group destination = %+v", d)
	}
}

// Codex r1 class trustee-object-id-unmatched: Exchange Online renders
// GrantSendOnBehalfTo as object GUIDs unless the display-name switch is used
// (Get-DistributionGroup / Get-Mailbox -IncludeGrantSendOnBehalfToWithDisplayNames),
// and a trustee can be rendered by any identifier the recipient carries
// (GUID, ExternalDirectoryObjectId, DistinguishedName, Identity, alias,
// address). Every one of them that is the user's must count.
func TestAccessMapMatchesTrusteeObjectIDs(t *testing.T) {
	const guid = "11111111-1111-4111-8111-111111111111"
	const oid = "22222222-2222-4222-8222-222222222222"
	const dn = "CN=jane,OU=contoso.onmicrosoft.com,OU=Microsoft Exchange Hosted Organizations,DC=NAMPR01A001,DC=prod,DC=outlook,DC=com"
	f := newFakeEXO(t, func(c string, p map[string]any) any {
		jane := map[string]any{"Name": "jane", "Alias": "jane", "DisplayName": "Jane Doe", "PrimarySmtpAddress": "jane@example.com",
			"Guid": guid, "ExternalDirectoryObjectId": oid, "DistinguishedName": dn, "Identity": "jane", "RecipientTypeDetails": "UserMailbox"}
		switch c {
		case "Get-Recipient":
			if f, _ := p["Filter"].(string); f != "" {
				return nil // no other recipient carries the rendered text as its display name
			}
			id := strings.ToLower(fmt.Sprint(p["Identity"]))
			for _, k := range []string{"jane@example.com", guid, oid, strings.ToLower(dn), "jane"} {
				if id == k {
					return []any{jane}
				}
			}
			return nil
		case "Get-Mailbox":
			if p["Identity"] != nil {
				return []any{map[string]any{"UserPrincipalName": "jane@example.com", "PrimarySmtpAddress": "jane@example.com", "Guid": guid}}
			}
			return []any{
				map[string]any{"Name": "ByGuid", "PrimarySmtpAddress": "byguid@example.com", "GrantSendOnBehalfTo": []any{guid}},
				map[string]any{"Name": "ByOid", "PrimarySmtpAddress": "byoid@example.com", "GrantSendOnBehalfTo": []any{strings.ToUpper(oid)}},
				map[string]any{"Name": "ByDN", "PrimarySmtpAddress": "bydn@example.com", "GrantSendOnBehalfTo": []any{dn}},
				map[string]any{"Name": "Other", "PrimarySmtpAddress": "other@example.com", "GrantSendOnBehalfTo": []any{"33333333-3333-4333-8333-333333333333"}},
			}
		}
		return nil
	})
	v := runEXO(t, "access-map", testTenant, "jane@example.com", "--skip-full-access")
	got := map[string]string{}
	for _, e := range v["access"].([]any) {
		m := e.(map[string]any)
		var parts []string
		for _, a := range m["access"].([]any) {
			parts = append(parts, a.(string))
		}
		got[m["mailbox"].(string)] = strings.Join(parts, ",")
	}
	for _, mbx := range []string{"byguid@example.com", "byoid@example.com", "bydn@example.com"} {
		if got[mbx] != "SendOnBehalf" {
			t.Errorf("%s: want SendOnBehalf from an object-ID trustee, got %q (access %v)", mbx, got[mbx], got)
		}
	}
	if _, ok := got["other@example.com"]; ok {
		t.Errorf("another recipient's GUID was counted as the user: %v", got)
	}
	_ = f
}
