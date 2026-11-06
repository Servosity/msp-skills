// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the read-write
// store opens SQLite through the credential kit's driver (one line in
// OpenWithContext), so a statement whose text or bound values carry a held
// credential does not run, on every table and from every writer.

package store

import (
	"database/sql"

	"afi-pp-cli/internal/credkit"

	"modernc.org/sqlite"
)

// KitSQLiteDriver is the database/sql driver name of the kit's SQLite wrapper.
const KitSQLiteDriver = "credkit-sqlite"

func init() {
	sql.Register(KitSQLiteDriver, credkit.SQLDriver(&sqlite.Driver{}))
}
