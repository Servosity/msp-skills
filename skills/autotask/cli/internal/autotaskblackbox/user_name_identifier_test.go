// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// The Autotask API user name is an identifier, not a held credential
// (credwire Declarations.Identifiers). It is the API user's login name, an
// email address in the MSP's own domain, and Autotask's answers repeat it and
// its domain (every Resource, most Contacts). Held, it withheld every answer
// that named a colleague at that domain, and a sync of such a record failed
// its upsert and stopped that resource's batch (kit finding 10). The
// integration code and the Secret stay held: the control below is withheld.
// A short user name that occurs in the program's own text (K7, kit finding
// 3) no longer refuses the start.

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestAPIUserNameIsAnIdentifier(t *testing.T) {
	t.Parallel()
	const (
		user      = "apiuser@contoso-msp.com"
		colleague = "tech@contoso-msp.com"
	)
	var mu sync.Mutex
	authed := 0
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		if r.Header.Get("ApiIntegrationCode") == bbCode && r.Header.Get("UserName") == user && r.Header.Get("Secret") == bbSecret {
			authed++
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch p := strings.TrimPrefix(r.URL.Path, apiBase); {
		case p == "/ConfigurationItems/query":
			fmt.Fprint(w, listBody(r, body, `{"id":31,"email":"`+colleague+`"}`))
		case p == "/Resources/query":
			// The API user is a Resource: its own record repeats its login name.
			fmt.Fprint(w, listBody(r, body, `{"id":29683481,"userName":"apiuser","email":"`+user+`","firstName":"API","lastName":"User","isActive":true}`))
		case p == "/Tickets/"+bbTicketID:
			// Control: an answer that repeats the Secret it was sent.
			fmt.Fprintf(w, `{"item":{"id":%s,"title":"HQ-Record","note":%q}}`, bbTicketID, r.Header.Get("Secret"))
		default:
			fmt.Fprint(w, listBody(r, body))
		}
	}))
	e := kitEnv(t, srv.URL+apiBase, caFileFor(t, srv), bbCode, user, bbSecret)

	query := e.run(t, "configuration-items", "query", "--filter", allFilter, "--json")
	if !strings.Contains(query, colleague) || strings.Contains(query, "credkit:") {
		t.Errorf("configuration-items query: want the record naming %s, got:\n%.1500s", colleague, query)
	}
	resources := e.run(t, "resources", "query", "--filter", allFilter, "--json")
	if !strings.Contains(resources, user) || strings.Contains(resources, "credkit:") {
		t.Errorf("resources query: want the API user's own Resource naming %s, got:\n%.1500s", user, resources)
	}
	mcp := e.mcpCalls(t, []map[string]any{{"name": "autotask_read", "arguments": map[string]any{
		"endpoint_id": "configuration-items.query", "params": map[string]any{"filter": []any{map[string]any{"op": "gte", "field": "id", "value": 0}}}}}})
	if !strings.Contains(mcp, colleague) || strings.Contains(mcp, "credkit:") {
		t.Errorf("MCP configuration-items.query: want the record naming %s, got:\n%.1500s", colleague, mcp)
	}

	synced := e.run(t, "sync", "--resources", "configuration-items", "--json")
	if strings.Contains(synced, "upserting") || strings.Contains(synced, "credkit:") {
		t.Errorf("sync: the record naming %s did not land:\n%.1500s", colleague, synced)
	}
	stored := false
	scanFiles(e.home, func(rel string, data []byte) {
		stored = stored || isStoreFile(rel) && strings.Contains(string(data), colleague)
	})
	if !stored {
		t.Errorf("sync: the local store holds no record naming %s:\n%.1500s", colleague, synced)
	}

	control := e.run(t, "tickets", "query-item", bbTicketID, "--json")
	if strings.Contains(control, bbSecret) || !strings.Contains(control, "credkit: withheld") {
		t.Errorf("control: an answer repeating the Secret must stay withheld, got:\n%.1500s", control)
	}
	short := kitEnv(t, srv.URL+apiBase, caFileFor(t, srv), bbCode, "K7", bbSecret)
	if out := short.run(t, "configuration-items", "query", "--filter", allFilter, "--json"); strings.Contains(out, "refusing to start") || !strings.Contains(out, colleague) {
		t.Errorf("a short user name: want the query answered, got:\n%.1500s", out)
	}
	mu.Lock()
	n := authed
	mu.Unlock()
	if n < 5 {
		t.Errorf("the fake saw the three credentials on %d request(s); the commands did not all authenticate", n)
	}
}
