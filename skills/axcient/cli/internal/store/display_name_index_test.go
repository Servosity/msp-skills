package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestSearchByThreeLetterUpperName_Issue148 is the regression oracle for issue
// #148: a record whose name is a 3-letter all-caps host label (WEB/SQL/DNS/DMZ)
// must be searchable by name. Before the fix, shouldIndexSearchString's
// key-blind 3-letter-all-caps branch dropped the name from the FTS index, so
// MCP search returned 0 rows for it while list/get (live API) still found it.
func TestSearchByThreeLetterUpperName_Issue148(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	items := []json.RawMessage{
		json.RawMessage(`{"id_": 1, "name": "WEB", "client_name": "Acme Corp"}`),
		json.RawMessage(`{"id_": 2, "name": "AcmeServer01"}`),
	}
	if _, _, err := s.UpsertBatch("device", items); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{"WEB", "web", "AcmeServer01"} {
		hits, err := s.SearchHits(q, 10)
		if err != nil {
			t.Fatalf("SearchHits(%q): %v", q, err)
		}
		if len(hits) == 0 {
			t.Errorf("search-by-name %q returned 0 results (issue #148: 3-letter all-caps name dropped from FTS)", q)
		}
	}

	// The display-name exemption must NOT re-admit bare 3-letter codes under a
	// non-name field: a currency code under "currency" stays out of the index.
	if shouldIndexSearchString("currency", "USD") {
		t.Errorf("3-letter all-caps code under a non-name key should still be dropped from the index")
	}
	// ...but a 3-letter all-caps value under a display-name key must be indexed.
	for _, k := range []string{"name", "alias", "hostname", "device_name"} {
		if !shouldIndexSearchString(k, "DMZ") {
			t.Errorf("3-letter all-caps value under display-name key %q must be indexed", k)
		}
	}
}
