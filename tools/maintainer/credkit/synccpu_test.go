// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// synccpu (owner GO 2026-11-09): the store passes through except one rule,
// a held vendor response value, raw or JSON-escaped, in a statement argument.

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const storeRow = `{"id":"7","notes":"Backup failed on SRV-FS07; see <a href=\"https://kb.example/1\">KB</a>"}`

// Nothing from a vendor response held: every statement is stored, a config
// credential in it included (the accepted risk), and the check takes no lock.
func TestSynccpuStorePassesThroughWithoutAResponseValue(t *testing.T) {
	holding(t, cred) // source "test": not a response field
	db, rec := kitmemDB(t)
	if err := kitmemRow(db, "7", []byte(`{"id":"7","token":"`+cred+`"}`)); err != nil {
		t.Fatalf("a row carrying a non-response credential was refused: %v", err)
	}
	// FTS-shaped statement, as the store's index update writes it.
	if _, err := db.Exec("INSERT INTO resources_fts(rowid, content) VALUES (?, ?)", 1, "pat.doe@contoso.example "+cred); err != nil {
		t.Fatalf("an FTS statement was refused: %v", err)
	}
	if len(rec.stmts) != 2 {
		t.Fatalf("%d statements reached the driver, want 2", len(rec.stmts))
	}
	// Zero cost: with the held set's lock taken, a statement still runs.
	done := make(chan error, 1)
	reg.mu.Lock()
	go func() { done <- kitmemRow(db, "8", []byte(storeRow)) }()
	select {
	case err := <-done:
		reg.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		reg.mu.Unlock()
		t.Fatal("a statement waited on the held set while no response value is held")
	}
	// With a response value held, the rule still reads only response values.
	Hold("response:token", "Unrelated-Resp-Value-91")
	if err := kitmemRow(db, "9", []byte(`{"id":"9","token":"`+cred+`"}`)); err != nil {
		t.Fatalf("a non-response credential was refused once a response value was held: %v", err)
	}
}

// A held response value is refused whole, raw or JSON-escaped, in a string
// or []byte argument, through a prepared or a direct statement; a row not
// carrying it is stored; the JSON words true and false never refuse a row.
func TestSynccpuStoreRefusesAHeldResponseValue(t *testing.T) {
	resetForTest(Declarations{})
	secret := `Rv9<Kq2&Lm4"Zx7`
	Hold("response:password", secret)
	Hold("response:active", "true")
	db, rec := kitmemDB(t)
	bs := string(rune(92))
	escaped := "Rv9" + bs + "u003cKq2" + bs + "u0026Lm4" + bs + `"Zx7` // encoding/json's default
	plain := `Rv9<Kq2&Lm4` + bs + `"Zx7`                               // SetEscapeHTML(false)
	for name, row := range map[string]string{"raw": "notes " + secret, "json": `{"password":"` + escaped + `"}`, "json no html escape": `{"p":"` + plain + `"}`} {
		if err := kitmemRow(db, "9", []byte(row)); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: not refused (%v)", name, err)
		}
		if _, err := db.Exec("INSERT INTO resources_fts(rowid, content) VALUES (?, ?)", 9, row); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s as a string: not refused (%v)", name, err)
		}
	}
	if err := kitmemRow(db, "10", []byte(`{"id":"10","active":true}`)); err != nil {
		t.Errorf("a row saying true was refused: %v", err)
	}
	if err := kitmemRow(db, "11", []byte(storeRow)); err != nil {
		t.Errorf("an ordinary row was refused: %v", err)
	}
	for _, s := range rec.stmts {
		if strings.Contains(s, "Rv9") {
			t.Fatalf("a statement carrying the value reached the driver: %q", s)
		}
	}
	if len(rec.stmts) != 2 {
		t.Fatalf("%d statements reached the driver, want the 2 clean ones", len(rec.stmts))
	}
}

// A held set made unknown (a response the kit could not read) refuses every
// statement with a value, as before (fail closed).
func TestSynccpuStoreFailsClosedWhenUnknown(t *testing.T) {
	resetForTest(Declarations{})
	db, _ := kitmemDB(t)
	markUnknown()
	if err := kitmemRow(db, "12", []byte(storeRow)); !errors.Is(err, ErrWithheld) {
		t.Fatalf("stored with the held set unknown: %v", err)
	}
}

// A held value that itself contains a JSON escape sequence (a literal
// backslash-u003c) is still refused in both JSON forms: the needles come from
// an encoder, not from rewriting the escaped text (Codex synccpu r1).
func TestSynccpuStoreRefusesAValueHoldingAnEscapeSequence(t *testing.T) {
	resetForTest(Declarations{})
	bs := string(rune(92))
	secret := "Rv9" + bs + "u003c<Kq2Zx7"
	Hold("response:password", secret)
	db, _ := kitmemDB(t)
	rows := map[string]string{
		"json":                `{"n":"Rv9` + bs + bs + "u003c" + bs + `u003cKq2Zx7"}`,
		"json no html escape": `{"n":"Rv9` + bs + bs + `u003c<Kq2Zx7"}`,
	}
	for name, row := range rows {
		if err := kitmemRow(db, "12", []byte(row)); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: not refused (%v)", name, err)
		}
	}
}
