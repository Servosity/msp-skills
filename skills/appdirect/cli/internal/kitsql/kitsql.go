// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the one SQLite driver this program opens, wrapped
// by the shared credential kit. Pinned by this connector's handfixes.json under
// the "credkit-wiring" entry.

// Package kitsql registers SQLite behind credkit.SQLDriver: every statement,
// its text and bound values, is checked before it runs.
package kitsql

import (
	"database/sql"

	"modernc.org/sqlite"

	"appdirect-pp-cli/internal/credkit"
)

// Driver is the name every sql.Open in this program uses.
const Driver = "credkit-sqlite"

func init() {
	sql.Register(Driver, credkit.SQLDriver(&sqlite.Driver{}))
}
