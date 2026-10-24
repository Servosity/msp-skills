// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 1 (reprint on press 4.34.0): each test below failed on
// the code Codex reviewed and names the class of defect it pins.
package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/store"
)

// rpcMock answers Login plus a caller-supplied handler for every other
// method, and counts calls per method.
type rpcMock struct {
	srv   *httptest.Server
	calls map[string]*atomic.Int32
}

func (m *rpcMock) count(method string) int32 {
	if c := m.calls[method]; c != nil {
		return c.Load()
	}
	return 0
}

func newRPCMock(t *testing.T, handle func(method string, params map[string]any) any) *rpcMock {
	t.Helper()
	m := &rpcMock{calls: map[string]*atomic.Int32{}}
	for _, name := range []string{"Login", "GetServerInfo", "RemoveAccount", "EnumerateAccountStatistics", "EnumerateAuditActions"} {
		m.calls[name] = &atomic.Int32{}
	}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if c := m.calls[req.Method]; c != nil {
			c.Add(1)
		}
		if req.Method == "Login" {
			fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"test-visa-0123456789","result":{"result":{"Id":1,"PartnerId":9000,"EmailAddress":"api@test"}}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "jsonrpc", "jsonrpc": "2.0", "visa": "test-visa-0123456789",
			"result": map[string]any{"result": handle(req.Method, req.Params)},
		})
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// Class: a remote write without confirmation. `call` sends only read methods
// (Get*, Enumerate*) unless --yes confirms; MCP can never pass --yes.
func TestCallRefusesAnUnconfirmedWrite(t *testing.T) {
	m := newRPCMock(t, func(string, map[string]any) any { return map[string]any{"ok": true} })
	fleetTestEnv(t, m.srv)
	if out, err := runCoveCmd(t, "call", "RemoveAccount", "--params", `{"accountId":42}`, "--agent"); err == nil {
		t.Fatalf("unconfirmed RemoveAccount succeeded:\n%s", out)
	}
	if n := m.count("RemoveAccount"); n != 0 {
		t.Fatalf("RemoveAccount was dispatched %d time(s) without --yes", n)
	}
	if out, err := runCoveCmd(t, "call", "GetServerInfo", "--agent"); err != nil {
		t.Fatalf("read method refused: %v\n%s", err, out)
	}
	if out, err := runCoveCmd(t, "call", "RemoveAccount", "--params", `{"accountId":42}`, "--yes", "--agent"); err != nil || m.count("RemoveAccount") != 1 {
		t.Fatalf("confirmed RemoveAccount: err=%v dispatched=%d\n%s", err, m.count("RemoveAccount"), out)
	}
}

// Class: server-controlled success data shown without the funnel.
func TestCallSuccessOutputNeverRepeatsThePassword(t *testing.T) {
	m := newRPCMock(t, func(string, map[string]any) any { return map[string]any{"Version": "pw-0123456789abc"} })
	fleetTestEnv(t, m.srv)
	t.Setenv("COVE_PASSWORD", "pw-0123456789abc")
	out, _ := runCoveCmd(t, "call", "GetServerInfo", "--json")
	if strings.Contains(out, "pw-0123456789abc") {
		t.Fatalf("call printed the password:\n%s", out)
	}
}

func statsRows(rows ...map[string]any) func(string, map[string]any) any {
	return func(method string, _ map[string]any) any { return rows }
}

// Class: one shared parser for Cove column values (multi-valued columns).
func TestDevicesSourcesKeepsEveryValueOfAMultiValuedColumn(t *testing.T) {
	m := newRPCMock(t, statsRows(map[string]any{"AccountId": 1, "Settings": []any{
		map[string]any{"I1": "srv-1"}, map[string]any{"I8": "Acme"}, map[string]any{"I78": []any{"D1", "D2"}},
	}}))
	fleetTestEnv(t, m.srv)
	out, err := runCoveCmd(t, "devices", "sources", "--without", "D2", "--json")
	if err != nil {
		t.Fatalf("devices sources: %v\n%s", err, out)
	}
	var view struct {
		Items   []map[string]any `json:"items"`
		Sources []struct {
			Code    string `json:"code"`
			Devices int    `json:"devices"`
		} `json:"sources"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(view.Items) != 0 {
		t.Fatalf("device protecting D2 listed as missing it: %v", view.Items)
	}
	d2 := 0
	for _, s := range view.Sources {
		if s.Code == "D2" {
			d2 = s.Devices
		}
	}
	if d2 != 1 {
		t.Fatalf("D2 tally = %d, want 1 (%s)", d2, out)
	}
}

// Class: one shared parser for Cove column values (JSON numbers), and
// aggregates computed from rounded display values.
func TestBillingUsageDecodesNumericColumns(t *testing.T) {
	m := newRPCMock(t, statsRows(
		map[string]any{"AccountId": 1, "Settings": []any{map[string]any{"I1": "a"}, map[string]any{"I14": 2000000000}}},
	))
	fleetTestEnv(t, m.srv)
	out, err := runCoveCmd(t, "billing", "usage", "--json")
	if err != nil {
		t.Fatalf("billing usage: %v\n%s", err, out)
	}
	var view struct {
		Items []struct {
			UsedStorageBytes int64 `json:"used_storage_bytes"`
		} `json:"items"`
		TotalStorageGB float64 `json:"total_storage_gb"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil || len(view.Items) != 1 {
		t.Fatalf("bad output: %v\n%s", err, out)
	}
	if view.Items[0].UsedStorageBytes != 2000000000 {
		t.Fatalf("numeric I14 decoded as %d bytes, want 2000000000", view.Items[0].UsedStorageBytes)
	}
}

func TestBillingUsageSumsBytesBeforeRounding(t *testing.T) {
	var view struct {
		TotalStorageGB float64 `json:"total_storage_gb"`
	}
	m2 := newRPCMock(t, statsRows(
		map[string]any{"AccountId": 1, "Settings": []any{map[string]any{"I1": "a"}, map[string]any{"I14": "15000000"}}},
		map[string]any{"AccountId": 2, "Settings": []any{map[string]any{"I1": "b"}, map[string]any{"I14": "15000000"}}},
	))
	fleetTestEnv(t, m2.srv)
	out, err := runCoveCmd(t, "billing", "usage", "--json")
	if err != nil {
		t.Fatalf("billing usage: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("bad output: %v\n%s", err, out)
	}
	if view.TotalStorageGB != 0.03 {
		t.Fatalf("total_storage_gb = %v, want 0.03 (30,000,000 bytes)", view.TotalStorageGB)
	}
}

// Class: one explicit status classification; adverse and unknown statuses
// are never healthy and never dropped.
func TestFleetHealthAndFailuresNeverTreatAnAdverseOrUnknownStatusAsHealthy(t *testing.T) {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	dev := func(id int, name, status string) map[string]any {
		return map[string]any{"AccountId": id, "Settings": settingsList(map[string]string{"I1": name, "I8": "Acme", "D9F00": status, "D9F09": now, "D9F15": now})}
	}
	m := newRPCMock(t, statsRows(dev(1, "no-selection", "11"), dev(2, "unrecognised", "13"), dev(3, "good", "5")))
	fleetTestEnv(t, m.srv)
	out, err := runCoveCmd(t, "fleet", "health", "--json")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, out)
	}
	var h struct {
		Total   int `json:"total_devices"`
		Healthy int `json:"healthy"`
		Failed  int `json:"failed"`
	}
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if h.Healthy != 1 || h.Failed < 1 {
		t.Fatalf("healthy=%d failed=%d, want healthy=1 (only status 5) and NoSelection counted failed\n%s", h.Healthy, h.Failed, out)
	}

	out, err = runCoveCmd(t, "devices", "failures", "--json")
	if err != nil {
		t.Fatalf("devices failures: %v\n%s", err, out)
	}
	for _, name := range []string{"no-selection", "unrecognised"} {
		if !strings.Contains(out, `"`+name+`"`) {
			t.Fatalf("devices failures dropped %s:\n%s", name, out)
		}
	}
	if strings.Contains(out, `"good"`) {
		t.Fatalf("devices failures listed a completed device:\n%s", out)
	}
}

func coveStoreRow(acct int64, status string) store.CoveDeviceStat {
	st, _ := strconv.ParseInt(status, 10, 64)
	return store.CoveDeviceStat{AccountID: acct, DeviceName: fmt.Sprintf("dev-%d", acct), Customer: "Acme", LastStatus: st,
		SettingsJSON: store.MarshalSettings(map[string]string{"I1": fmt.Sprintf("dev-%d", acct), "D9F00": status})}
}

// Class: a status that is merely no longer adverse is not a recovery; a
// recovery needs a completed session.
func TestDevicesChangesRecoveryNeedsACompletedSession(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := filepath.Join(t.TempDir(), "cove.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-2 * time.Hour)
	// A known scope: snapshots with no recorded endpoint are never compared,
	// which would make this test pass without judging the flip at all.
	scope := store.CoveSnapshotScope{RootPartnerID: 9000, Endpoint: "POST https://api.backup.management/jsonapi", Complete: true}
	if _, err := db.InsertCoveSnapshotScoped(t.Context(), t0, scope, []store.CoveDeviceStat{coveStoreRow(1, "2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertCoveSnapshotScoped(t.Context(), t0.Add(time.Hour), scope, []store.CoveDeviceStat{coveStoreRow(1, "1")}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err := runCoveCmd(t, "devices", "changes", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	if strings.Contains(out, `"recovery"`) || !strings.Contains(out, `"change"`) {
		t.Fatalf("Failed -> InProcess must be a change, not a recovery:\n%s", out)
	}
}

// pagingFleet serves EnumerateAccountStatistics per PartnerId, honoring
// StartRecordNumber/RecordsCount.
func pagingFleet(byPartner map[int64][]map[string]any) func(string, map[string]any) any {
	return func(method string, params map[string]any) any {
		q, _ := params["query"].(map[string]any)
		pid, _ := q["PartnerId"].(float64)
		start, _ := q["StartRecordNumber"].(float64)
		count, _ := q["RecordsCount"].(float64)
		rows := byPartner[int64(pid)]
		lo, hi := int(start), int(start)+int(count)
		if lo > len(rows) {
			lo = len(rows)
		}
		if hi > len(rows) {
			hi = len(rows)
		}
		return rows[lo:hi]
	}
}

func fleetRow(id int, name string) map[string]any {
	return map[string]any{"AccountId": id, "Settings": settingsList(map[string]string{"I1": name, "I8": "Acme", "D9F00": "5", "I14": "100"})}
}

func backdateSnapshots(t *testing.T, dbPath string, by time.Duration) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE cove_snapshots SET taken_at = taken_at - ?`, int64(by.Seconds())); err != nil {
		t.Fatal(err)
	}
}

// Class: snapshot comparisons across different scopes or across a partial
// capture report devices added and removed that never changed.
func TestDevicesChangesComparesOnlyLikeSnapshots(t *testing.T) {
	m := newRPCMock(t, pagingFleet(map[int64][]map[string]any{
		100: {fleetRow(1, "dev-A")},
		200: {fleetRow(2, "dev-B")},
	}))
	fleetTestEnv(t, m.srv)

	scopeDB := filepath.Join(t.TempDir(), "scope.db")
	if out, err := runCoveCmd(t, "snapshot", "--partner-id", "100", "--db", scopeDB, "--json"); err != nil {
		t.Fatalf("snapshot 100: %v\n%s", err, out)
	}
	backdateSnapshots(t, scopeDB, time.Minute)
	if out, err := runCoveCmd(t, "snapshot", "--partner-id", "200", "--db", scopeDB, "--json"); err != nil {
		t.Fatalf("snapshot 200: %v\n%s", err, out)
	}
	out, err := runCoveCmd(t, "devices", "changes", "--db", scopeDB, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	if strings.Contains(out, "dev-A") || strings.Contains(out, "dev-B") {
		t.Fatalf("snapshots of different partners were compared:\n%s", out)
	}
}

func TestDevicesChangesIgnoresDevicesOutsideAPartialSnapshot(t *testing.T) {
	m := newRPCMock(t, pagingFleet(map[int64][]map[string]any{
		300: {fleetRow(3, "dev-C"), fleetRow(4, "dev-D")},
	}))
	fleetTestEnv(t, m.srv)
	partialDB := filepath.Join(t.TempDir(), "partial.db")
	if out, err := runCoveCmd(t, "snapshot", "--partner-id", "300", "--db", partialDB, "--json"); err != nil {
		t.Fatalf("full snapshot: %v\n%s", err, out)
	}
	backdateSnapshots(t, partialDB, time.Minute)
	if out, err := runCoveCmd(t, "snapshot", "--partner-id", "300", "--page-size", "1", "--max-scan-pages", "1", "--db", partialDB, "--json"); err != nil {
		t.Fatalf("partial snapshot: %v\n%s", err, out)
	}
	out, err := runCoveCmd(t, "devices", "changes", "--db", partialDB, "--json")
	if err != nil {
		t.Fatalf("devices changes: %v\n%s", err, out)
	}
	if strings.Contains(out, "dev-D") {
		t.Fatalf("device outside a partial snapshot reported removed:\n%s", out)
	}
}

// P2 (data-source strategy): a live command refuses --data-source local
// instead of dialling Cove.
func TestHandBuiltLiveCommandsRefuseDataSourceLocal(t *testing.T) {
	m := newRPCMock(t, statsRows())
	fleetTestEnv(t, m.srv)
	for _, args := range [][]string{
		{"devices", "sources"}, {"devices", "failures"}, {"devices", "stale"}, {"fleet", "health"},
		{"fleet", "activity"}, {"billing", "usage"}, {"billing", "changes"}, {"call", "GetServerInfo"},
	} {
		if out, err := runCoveCmd(t, append(args, "--data-source", "local", "--json")...); err == nil {
			t.Fatalf("%v --data-source local succeeded:\n%s", args, out)
		}
	}
	if n := m.count("Login") + m.count("EnumerateAccountStatistics") + m.count("GetServerInfo") + m.count("EnumerateAuditActions"); n != 0 {
		t.Fatalf("%d live request(s) made under --data-source local", n)
	}
}

// Class: credentials shorter than the shared funnel's decoded-view minimum
// bypassed the CLI error backstop.
// The shared backstop (funnelError, funnelShownError) stays byte-identical
// across connectors, so cove composes its short-credential backstop with it:
// coveShortFunnelError after funnelError on a command's error (as Execute
// wires them), and before funnelShownError in the JSON error envelope.
func TestCLIBackstopWithholdsAShortHeldCredential(t *testing.T) {
	cliutil.FunnelHold("Zq7#x")
	err := coveShortFunnelError(funnelError(errors.New("upstream said Zq7#x was wrong")))
	if strings.Contains(err.Error(), "Zq7#x") {
		t.Fatalf("short held credential shown: %v", err)
	}
	var envelope strings.Builder
	writeAPIErrorEnvelope(&envelope, &rootFlags{asJSON: true}, errors.New("token Zq7#x"), 1)
	if strings.Contains(envelope.String(), "Zq7#x") || !strings.Contains(envelope.String(), "withheld") {
		t.Fatalf("short held credential shown in envelope: %s", envelope.String())
	}
}

// P2 (cheap correctness): D9 is Cove's synthetic Total data source, never an
// active source; --without D9 would list every device as unprotected.
func TestDevicesSourcesRefusesTheSyntheticTotalSource(t *testing.T) {
	for _, v := range []string{"D9", "Total", "d09"} {
		if _, err := resolveDataSource(v); err == nil {
			t.Fatalf("--without %s accepted", v)
		}
	}
	if code, err := resolveDataSource("system state"); err != nil || code != "D2" {
		t.Fatalf("system state -> %q %v", code, err)
	}
}
