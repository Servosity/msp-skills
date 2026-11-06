// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"reflect"
	"testing"
)

// reviewedImportTargets is every resource `import` may POST records to, each a
// reviewed create endpoint. The press picks a resource's first POST as its
// write path, which for org was the bulk clear-owners action: `import org`
// would have cleared asset owners using each record as the search filter. A
// reprint that adds or changes a mapping fails here until it is reviewed.
var reviewedImportTargets = map[string]string{
	"account": "/account/assets/ownership-types", // createAssetOwnershipTypes
}

func TestImportTargetsOnlyReviewedCreateEndpoints(t *testing.T) {
	if !reflect.DeepEqual(resourceWritePaths, reviewedImportTargets) {
		t.Fatalf("import write paths = %v, want only the reviewed create endpoints %v", resourceWritePaths, reviewedImportTargets)
	}
	if path, err := resourceWritePath("org"); err == nil {
		t.Fatalf("import org resolves to %s; it must be refused", path)
	}
}
