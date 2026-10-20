// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

// Package dsmtest holds helpers for tests that stand in for a DSM.
package dsmtest

import (
	"io"
	"net/http"
)

// AuthInfo is DSM's answer to the unauthenticated login probe
// (dsm.APIInfoProbePath), as DSM 6 and 7 send it.
const AuthInfo = `{"data":{"SYNO.API.Auth":{"maxVersion":7,"minVersion":1,"path":"entry.cgi"}},"success":true}`

// ServeAPIInfo answers the login probe (SYNO.API.Info query=SYNO.API.Auth on
// /webapi/query.cgi) the way a real DSM does and reports whether it did, so
// a fake NAS handler can start with
//
//	if dsmtest.ServeAPIInfo(w, r) {
//		return
//	}
//
// Every other request (including SYNO.API.Info query=all) is left to the fake.
func ServeAPIInfo(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	if r.Method != http.MethodGet || r.URL.Path != "/webapi/query.cgi" || q.Get("api") != "SYNO.API.Info" || q.Get("query") != "SYNO.API.Auth" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, AuthInfo)
	return true
}
