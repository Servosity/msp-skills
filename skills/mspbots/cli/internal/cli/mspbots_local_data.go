// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// mspbots handfix local-data-off-covers-snapshot: `snapshot` copies dataset
// and widget rows into the local store, so MSPBOTS_NO_LOCAL_DATA=1 refuses it
// like the generated store writers (TestNoLocalDataRefusesSnapshot).
func init() { localDataWriterCommands["snapshot"] = true }
