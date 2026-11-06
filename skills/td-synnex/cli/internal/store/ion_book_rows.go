// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json: ion-book-one-account): every licence-book row
// is written only for the ION account its list is recorded for, in the same
// transaction that commits it, and is tagged with that account.

package store

import (
	"database/sql"
	"fmt"
	"sync"
)

// IONBookRowDDL records, per book row, the ION account whose write committed it.
const IONBookRowDDL = `CREATE TABLE IF NOT EXISTS ion_book_row (resource_type TEXT NOT NULL, id TEXT NOT NULL, account TEXT NOT NULL, PRIMARY KEY (resource_type, id))`

// IsIONBookList reports whether resource is one of the lists the licence book reads.
func IsIONBookList(resource string) bool {
	return resource == "customers" || resource == "subscriptions" || resource == "orders"
}

// IONBookAccount names the account a writer that declared no walk writes for:
// a live read's or a mutation's write-back into the configured account's
// store. The cli sets it to the configured account.
var IONBookAccount = func() string { return "" }

type ionBookWalkKey struct {
	s        *Store
	resource string
}

type ionBookWalk struct{ account, scope string }

var ionBookWalks sync.Map // ionBookWalkKey -> ionBookWalk

// DeclareIONBookWalk records that this handle's sync walks resource for
// account under the scope it bound. Until EndIONBookWalk, each row it writes
// commits only while the list is still recorded for account and sync_state
// still holds scopeKey, so a walk whose claim another account's sync took
// over writes nothing.
func (s *Store) DeclareIONBookWalk(resource, account, scopeKey string) {
	ionBookWalks.Store(ionBookWalkKey{s, resource}, ionBookWalk{account, scopeKey})
}

// EndIONBookWalk drops the declaration DeclareIONBookWalk made.
func (s *Store) EndIONBookWalk(resource string) {
	ionBookWalks.Delete(ionBookWalkKey{s, resource})
}

// ionBookRowTx runs inside the transaction that writes resource/id: a book
// row is refused (the caller's transaction rolls back) unless the list's
// ion_store_account record names the writer's account and, for a declared
// walk, sync_state still holds the walk's scope; a written row is tagged
// with the account. A declared walk refuses on any read error.
func (s *Store) ionBookRowTx(tx *sql.Tx, resourceType, id string) error {
	if !IsIONBookList(resourceType) {
		return nil
	}
	v, declared := ionBookWalks.Load(ionBookWalkKey{s, resourceType})
	walk, _ := v.(ionBookWalk)
	if !declared {
		walk.account = IONBookAccount()
	}
	var owner string
	if err := tx.QueryRow(`SELECT account FROM ion_store_account WHERE resource_type = ?`, resourceType).Scan(&owner); err != nil && !declared {
		// No sync has claimed the list: the row commits untagged, and the book
		// refuses an untagged row until a sync claims and re-walks the list.
		if _, err := tx.Exec(IONBookRowDDL); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM ion_book_row WHERE resource_type = ? AND id = ?`, resourceType, id)
		return err
	}
	if walk.account == "" || owner != walk.account {
		return fmt.Errorf("%s in this store are not recorded for ION account %q (recorded: %q; another account's sync claimed the list), so nothing was written", resourceType, walk.account, owner)
	}
	if declared {
		var scope string
		if err := tx.QueryRow(`SELECT scope_key FROM sync_state WHERE resource_type = ?`, resourceType).Scan(&scope); err != nil || scope != walk.scope {
			return fmt.Errorf("sync scope for %s changed during the walk (another sync rebound it), so nothing was written", resourceType)
		}
	}
	_, err := tx.Exec(`INSERT INTO ion_book_row (resource_type, id, account) VALUES (?, ?, ?) ON CONFLICT(resource_type, id) DO UPDATE SET account = excluded.account`, resourceType, id, walk.account)
	return err
}
