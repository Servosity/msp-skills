// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kaseya-bms-pp-cli/internal/store"
)

// runKbmsAt runs the root command with a fixed home, so several commands in
// one test share one default store.
func runKbmsAt(t *testing.T, home string, env map[string]string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("KASEYA_BMS_HOME", home)
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	for _, k := range kbmsCredentialVars {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	var out, errb bytes.Buffer
	root := newRootCmd(&rootFlags{})
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

// Codex review r1 class "wrong local partition" (kaseya-bms handfix
// local-partition-follows-the-path): every generated servicedesk command
// names the one `servicedesk` partition, which sync fills with
// /v2/servicedesk/tickets (the partition queue-health, stale-tickets,
// workload and contract-burn read as tickets). A default (auto) live read of a
// hardware asset, a ticket's CC list or a ticket template would cache its
// record there, and the ticket novels would count it as a ticket; a
// mutation's answer would land there too. A sync partition now serves and
// receives only the records of its own path, and a local read of another
// servicedesk path is refused rather than answered with tickets.
func TestSyncPartitionHoldsOnlyItsOwnPath(t *testing.T) {
	inHoldingChild(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/servicedesk/tickets":
			_, _ = w.Write([]byte(`{"Success":true,"Error":null,"Result":[{"Id":11,"TicketNumber":"T20261029.0011","Title":"Printer offline",` +
				`"StatusName":"New","QueueName":"Help Desk","PriorityName":"High","AccountName":"Acme"}],"TotalRecords":1}`))
		case "/v2/servicedesk/hardwareasset/55":
			_, _ = w.Write([]byte(kbmsEnvelope(`{"Id":55,"Name":"PC1-HOST","AssetNumber":"HW-55","AccountName":"Acme"}`)))
		case "/v2/servicedesk/tickets/ticketCcs/7":
			_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":71,"Name":"CC-Contact","Email":"cc@acme.example"}]`)))
		case "/v2/servicedesk/templates/tickets/9":
			_, _ = w.Write([]byte(kbmsEnvelope(`{"Id":91,"Name":"Onboarding-Template","Title":"New hire"}`)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	// The token shares no 8-character piece with the test's name: the kit
	// holds every such piece of it, and the temp home's path carries the name.
	env := map[string]string{"KASEYA_BMS_BASE_URL": srv.URL, "KASEYA_BMS_TOKEN": "zq7Vm2Lw-" + "81xr-4c4c"}
	if _, stderr, err := runKbmsAt(t, home, env, "sync", "--resources", "servicedesk", "--full", "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	for _, args := range [][]string{
		{"servicedesk", "get-hardware-asset", "55", "--json"},
		{"servicedesk", "get-ccs", "7", "--json"},
		{"servicedesk", "get-ticket-template", "9", "--json"},
		{"servicedesk", "get-tickets", "--json"},
	} {
		if _, stderr, err := runKbmsAt(t, home, env, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr)
		}
	}
	writeMutationResponseToStore(context.Background(), "servicedesk", json.RawMessage(`{"Id":32,"Title":"Created-By-A-Post","TicketNumber":"T-POSTED"}`), "")

	db, err := store.Open(defaultDBPath("kaseya-bms-cli"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := kbmsRows(context.Background(), db, "servicedesk")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || kbmsStr(rows[0], "TicketNumber") != "T20261029.0011" {
		var got []string
		for _, r := range rows {
			got = append(got, kbmsStr(r, "TicketNumber")+"/"+kbmsStr(r, "Name"))
		}
		t.Errorf("the tickets partition holds records of other paths: %v", got)
	}
	stdout, stderr, err := runKbmsAt(t, home, env, "queue-health", "--json")
	if err != nil {
		t.Fatalf("queue-health: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "Help Desk") {
		t.Errorf("queue-health did not read the synced ticket:\n%s", stdout)
	}

	stdout, stderr, err = runKbmsAt(t, home, env, "servicedesk", "get-tickets", "--data-source", "local", "--json")
	if err != nil {
		t.Fatalf("local tickets: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "T20261029.0011") || strings.Contains(stdout, "PC1-HOST") || strings.Contains(stdout, "CC-Contact") || strings.Contains(stdout, "Onboarding-Template") {
		t.Errorf("a local read of /v2/servicedesk/tickets did not return only the synced tickets:\n%s", stdout)
	}
	stdout, stderr, err = runKbmsAt(t, home, env, "servicedesk", "get-hardware-asset", "55", "--data-source", "local", "--json")
	if err == nil || strings.Contains(stdout, "T20261029.0011") {
		t.Errorf("a local read of a hardware asset answered from the tickets partition (err %v):\n%s%s", err, stdout, stderr)
	}
}
