package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// Issue #464: shouldIndexSearchString dropped every 3-letter ALL-CAPS
// value (WEB, SQL, DNS), so a record named that never reached
// resources_fts and search returned 0 rows. The press removed the rule in
// v4.25.1 (e2a9b501); this connector carries the same deletion, plus a
// schema bump so stores built by the old binary rebuild their index.

// ftsThreeLetterCapsPreFixStoreSchemaVersion is the StoreSchemaVersion the
// binary that dropped 3-letter ALL-CAPS values stamped into its stores.
const ftsThreeLetterCapsPreFixStoreSchemaVersion = 4

const ftsThreeLetterCapsProbeType = "fts_caps_probe"

func TestShouldIndexSearchString_ThreeLetterAllCaps(t *testing.T) {
	for _, key := range []string{"name", "hostname", "role"} {
		for _, value := range []string{"WEB", "SQL", "DNS"} {
			if !shouldIndexSearchString(key, value) {
				t.Errorf("shouldIndexSearchString(%q, %q) = false, want true", key, value)
			}
		}
	}
}

func TestSearch_FindsThreeLetterAllCapsRecord(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	for i, name := range []string{"WEB", "SQL", "DNS"} {
		id := fmt.Sprintf("caps-%d", i)
		if err := s.Upsert(ftsThreeLetterCapsProbeType, id, []byte(fmt.Sprintf(`{"id":%q,"name":%q}`, id, name))); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}
	for _, name := range []string{"WEB", "SQL", "DNS"} {
		results, err := s.Search(name, 10)
		if err != nil {
			t.Fatalf("search %s: %v", name, err)
		}
		if len(results) != 1 {
			t.Errorf("search %q returned %d rows, want 1", name, len(results))
		}
	}
}

func TestOpen_RebuildsFTSForThreeLetterAllCapsOnOldStore(t *testing.T) {
	if resourcesFTSContentSchemaVersion <= ftsThreeLetterCapsPreFixStoreSchemaVersion {
		t.Fatalf("resourcesFTSContentSchemaVersion = %d; a store stamped %d by the old binary would never rebuild resources_fts",
			resourcesFTSContentSchemaVersion, ftsThreeLetterCapsPreFixStoreSchemaVersion)
	}
	if resourcesFTSContentSchemaVersion > StoreSchemaVersion {
		t.Fatalf("resourcesFTSContentSchemaVersion = %d > StoreSchemaVersion = %d; resources_fts would rebuild on every open",
			resourcesFTSContentSchemaVersion, StoreSchemaVersion)
	}

	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := s.Upsert(ftsThreeLetterCapsProbeType, "probe-1", []byte(`{"id":"probe-1","name":"DNS"}`)); err != nil {
		s.Close()
		t.Fatalf("seed upsert: %v", err)
	}
	// Reproduce what the old binary left on disk: the record is in
	// resources, its resources_fts row has no "DNS", and user_version is
	// the old binary's stamp.
	if _, err := s.DB().Exec(`UPDATE resources_fts SET content = '' WHERE rowid = ?`, ftsRowID(ftsThreeLetterCapsProbeType, "probe-1")); err != nil {
		s.Close()
		t.Fatalf("simulate old index: %v", err)
	}
	if _, err := s.DB().Exec(fmt.Sprintf(`PRAGMA user_version = %d`, ftsThreeLetterCapsPreFixStoreSchemaVersion)); err != nil {
		s.Close()
		t.Fatalf("stamp old version: %v", err)
	}
	s.Close()

	ro, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	results, err := ro.Search("DNS", 10)
	ro.Close()
	if err != nil {
		t.Fatalf("read-only search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("read-only open found %d rows before the rebuild; the simulated old index is not old", len(results))
	}

	s, err = Open(dbPath)
	if err != nil {
		t.Fatalf("upgrade open: %v", err)
	}
	v, err := s.SchemaVersion()
	if err != nil {
		s.Close()
		t.Fatalf("schema version: %v", err)
	}
	if v != StoreSchemaVersion {
		s.Close()
		t.Fatalf("upgraded version = %d, want %d", v, StoreSchemaVersion)
	}
	results, err = s.Search("DNS", 10)
	if err != nil {
		s.Close()
		t.Fatalf("search after upgrade: %v", err)
	}
	if len(results) != 1 {
		s.Close()
		t.Fatalf("search \"DNS\" after a read-write open of a v%d store returned %d rows, want 1 (resources_fts was not rebuilt)",
			ftsThreeLetterCapsPreFixStoreSchemaVersion, len(results))
	}
	// The rebuild runs once: a second open of the upgraded store keeps
	// whatever resources_fts holds.
	if _, err := s.DB().Exec(`UPDATE resources_fts SET content = 'sentinel' WHERE rowid = ?`, ftsRowID(ftsThreeLetterCapsProbeType, "probe-1")); err != nil {
		s.Close()
		t.Fatalf("write sentinel: %v", err)
	}
	s.Close()
	s, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()
	var content string
	if err := s.DB().QueryRow(`SELECT content FROM resources_fts WHERE rowid = ?`, ftsRowID(ftsThreeLetterCapsProbeType, "probe-1")).Scan(&content); err != nil {
		t.Fatalf("read content: %v", err)
	}
	if content != "sentinel" {
		t.Fatalf("resources_fts content = %q after a second open, want the sentinel kept (rebuild repeated)", content)
	}
}
