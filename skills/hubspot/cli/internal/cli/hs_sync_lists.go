// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "hubspot-pp-cli/internal/store"

// hsForgetDroppedSyncResources removes the checkpoint of hubspot-lists-crm,
// which sync no longer walks: GET /crm/v3/lists/idmapping translates one legacy
// list ID and answered HTTP 400 when called as a list, so its row can never
// finish and would hold store_status at partial for good.
// Hand-wired (handfixes.json: sync-never-calls-lists-idmapping).
func hsForgetDroppedSyncResources(db *store.Store) error {
	_, err := db.DB().Exec(`DELETE FROM sync_state WHERE resource_type = 'hubspot-lists-crm'`)
	return err
}
