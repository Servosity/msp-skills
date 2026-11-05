// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- class: inventory narrowed by a documented default filter -------------

// directory_v1.json roleAssignments.list: includeIndirectRoleAssignments
// "Defaults to false"; a role held through a group is returned only when it
// is true (and userKey is set).
func TestLeaverAdminRolesIncludeGroupDerivedRoles(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("includeIndirectRoleAssignments") == "true" && r.URL.Query().Get("userKey") == "u1" {
			_, _ = io.WriteString(w, `{"kind":"admin#directory#roleAssignments","items":[{"kind":"admin#directory#roleAssignment","roleAssignmentId":"9","roleId":"3","assignedTo":"g1","assigneeType":"group","scopeType":"CUSTOMER"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"kind":"admin#directory#roleAssignments","items":[]}`)
	}))
	defer srv.Close()
	c := testClient(t, srv.URL)
	n, err := leaverAdminRoleCount(context.Background(), c, gwsTenant{Name: "acme"}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("a suspended user's group-derived admin role must be counted: got %d (queries %q)", n, seen)
	}
}
