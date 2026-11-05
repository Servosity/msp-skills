// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// triage ranks what sync really stores: the threats list answers only
// {threatId} (spec.json GET /threats) and the detail walker fetches GET
// /threats/{threat_id} (messages[] per spec.json ThreatDetails), so a threat's
// rank must come from all of its messages, through the real binary.
func TestTriageRanksThreatsSyncedThroughTheDetailWalker(t *testing.T) {
	recv := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	msg := func(threat, id, attack, status string) string {
		return fmt.Sprintf(`{"threatId":%q,"abxMessageId":%s,"abxMessageIdStr":%q,"subject":"Invoice %s","fromAddress":"billing@attacker.example.net",`+
			`"receivedTime":%q,"attackType":%q,"attackStrategy":"Name Impersonation","attackVector":"Text","remediationStatus":%q}`,
			threat, id, id, id, recv, attack, status)
	}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v1") {
		case "/threats":
			fmt.Fprint(w, `{"threats":[{"threatId":"t-open"},{"threatId":"t-done"}],"pageNumber":1}`)
		case "/threats/t-open":
			fmt.Fprintf(w, `{"threatId":"t-open","recipientCount":2,"tenantId":1,"tenantName":"HQ","messages":[%s,%s],"pageNumber":1}`,
				msg("t-open", "4551618356913732076", "Extortion", "Not Remediated"), msg("t-open", "4551618356913732077", "Extortion", "Auto-Remediated"))
		case "/threats/t-done":
			fmt.Fprintf(w, `{"threatId":"t-done","recipientCount":1,"tenantId":1,"tenantName":"HQ","messages":[%s],"pageNumber":1}`,
				msg("t-done", "4551618356913732078", "Spam", "Remediated"))
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{}`)
		}
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, &server{headers: map[string]int{}, url: srv.URL + "/v1", caFile: ca}, credEnv)
	syncOut := e.run(t, "sync", "--resources", "threats", "--no-learn")
	out := e.run(t, "triage", "--since", "7d", "--json", "--no-learn")
	var view struct {
		Items []struct {
			ThreatID     string `json:"threatId"`
			AttackType   string `json:"attackType"`
			MessageCount int    `json:"messageCount"`
			Remediated   bool   `json:"remediated"`
		} `json:"items"`
		Scanned    int `json:"scanned_threats"`
		ListedLast int `json:"remediated_listed_last"`
	}
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&view); err != nil {
		t.Fatalf("triage output: %v\n%s\nsync:\n%s", err, out, syncOut)
	}
	if view.Scanned != 2 || view.ListedLast != 1 || len(view.Items) != 2 {
		t.Fatalf("want 2 scanned, both listed, the remediated one last:\n%s\nsync:\n%s", out, syncOut)
	}
	if it := view.Items[1]; it.ThreatID != "t-done" || !it.Remediated {
		t.Fatalf("t-done (every synced message remediated) must be listed after t-open: %+v\n%s", it, out)
	}
	if it := view.Items[0]; it.ThreatID != "t-open" || it.AttackType != "Extortion" || it.MessageCount != 2 || it.Remediated {
		t.Fatalf("t-open must be ranked from both synced messages: %+v\n%s", it, out)
	}
}
